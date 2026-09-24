locals {
  name_prefix = "cbpa-${var.environment}"
  tags = {
    Environment = var.environment
  }
}

# --- Networking ------------------------------------------------------

module "network" {
  source = "../../modules/network"

  name_prefix        = local.name_prefix
  vpc_cidr           = var.vpc_cidr
  single_nat_gateway = false # prod: one NAT gateway per AZ — an AZ's NAT failing shouldn't take down every private subnet's egress
  tags               = local.tags
}

module "security" {
  source = "../../modules/security"

  name_prefix = local.name_prefix
  vpc_id      = module.network.vpc_id
  tags        = local.tags
}

# --- Compute -----------------------------------------------------------

module "ecs_cluster" {
  source = "../../modules/ecs-cluster"

  name_prefix               = local.name_prefix
  enable_container_insights = true
  log_retention_days        = 90
  tags                      = local.tags
}

module "alb" {
  source = "../../modules/alb"

  name_prefix                = local.name_prefix
  vpc_id                     = module.network.vpc_id
  public_subnet_ids          = module.network.public_subnet_ids
  security_group_id          = module.security.alb_sg_id
  acm_certificate_arn        = var.acm_certificate_arn
  enable_deletion_protection = true # prod only — the public entry point shouldn't disappear via a stray `terraform destroy`
  tags                       = local.tags
}

# --- Data layer ----------------------------------------------------------

module "rds_mariadb" {
  source = "../../modules/rds-mariadb"

  name_prefix           = local.name_prefix
  private_subnet_ids    = module.network.private_subnet_ids
  security_group_id     = module.security.rds_sg_id
  instance_class        = "db.t4g.medium"
  allocated_storage_gb  = 100
  multi_az              = true # automatic standby failover
  backup_retention_days = 7
  is_production         = true
  tags                  = local.tags
}

module "clickhouse" {
  source = "../../modules/clickhouse-ec2"

  name_prefix         = local.name_prefix
  private_subnet_id   = module.network.private_subnet_ids[0]
  availability_zone   = module.network.availability_zones[0]
  security_group_id   = module.security.clickhouse_sg_id
  instance_type       = "r6g.large" # memory-optimized — ClickHouse benefits from RAM for query performance more than raw vCPU count
  data_volume_size_gb = 500

  tags = local.tags
}

module "msk" {
  source = "../../modules/msk"

  name_prefix        = local.name_prefix
  private_subnet_ids = module.network.private_subnet_ids
  security_group_id  = module.security.msk_sg_id
  tags               = local.tags
}

# --- Observability -------------------------------------------------------

module "observability" {
  source = "../../modules/observability"

  name_prefix = local.name_prefix
  tags        = local.tags
}

resource "aws_ssm_parameter" "otel_config_payments_service" {
  name = "/${local.name_prefix}/otel/payments-service"
  type = "String"
  value = templatefile("${path.module}/otel-collector-config.yaml.tftpl", {
    job_name              = "payments-service"
    port                  = 8080
    remote_write_endpoint = module.observability.amp_remote_write_endpoint
    aws_region            = var.aws_region
  })
}

resource "aws_ssm_parameter" "otel_config_payment_processor" {
  name = "/${local.name_prefix}/otel/payment-processor"
  type = "String"
  value = templatefile("${path.module}/otel-collector-config.yaml.tftpl", {
    job_name              = "payment-processor"
    port                  = 9091
    remote_write_endpoint = module.observability.amp_remote_write_endpoint
    aws_region            = var.aws_region
  })
}

resource "aws_ssm_parameter" "otel_config_analytics_service" {
  name = "/${local.name_prefix}/otel/analytics-service"
  type = "String"
  value = templatefile("${path.module}/otel-collector-config.yaml.tftpl", {
    job_name              = "analytics-service"
    port                  = 8082
    remote_write_endpoint = module.observability.amp_remote_write_endpoint
    aws_region            = var.aws_region
  })
}

data "aws_iam_policy_document" "amp_remote_write" {
  statement {
    actions   = ["aps:RemoteWrite", "aps:GetSeries", "aps:GetLabels", "aps:GetMetricMetadata"]
    resources = [module.observability.amp_workspace_arn]
  }
}

# See dev/main.tf's identical policy for why topic/group resources are
# scoped by type+region rather than to this exact cluster's topics.
data "aws_iam_policy_document" "msk_iam_auth" {
  statement {
    actions   = ["kafka-cluster:Connect", "kafka-cluster:DescribeCluster"]
    resources = [module.msk.cluster_arn]
  }
  statement {
    actions = [
      "kafka-cluster:ReadData",
      "kafka-cluster:WriteData",
      "kafka-cluster:DescribeTopic",
      "kafka-cluster:CreateTopic",
    ]
    resources = ["arn:aws:kafka:${var.aws_region}:*:topic/*"]
  }
  statement {
    actions   = ["kafka-cluster:AlterGroup", "kafka-cluster:DescribeGroup"]
    resources = ["arn:aws:kafka:${var.aws_region}:*:group/*"]
  }
}

data "aws_iam_policy_document" "combined_task_policy" {
  source_policy_documents = [
    data.aws_iam_policy_document.amp_remote_write.json,
    data.aws_iam_policy_document.msk_iam_auth.json,
  ]
}

# --- Application services ------------------------------------------------
# desired_count = 2 throughout (vs. 1 in dev/staging): prod runs at
# least two task instances per service so a single task's replacement
# (deploy, crash, AZ blip) never drops a service to zero capacity.

module "payments_service" {
  source = "../../modules/ecs-service"

  name_prefix           = local.name_prefix
  service_name          = "payments-service"
  aws_region            = var.aws_region
  cluster_id            = module.ecs_cluster.cluster_id
  log_group_name        = module.ecs_cluster.log_group_name
  container_port        = 8080
  cpu                   = 1024
  memory                = 2048
  desired_count         = 2
  subnet_ids            = module.network.private_subnet_ids
  security_group_id     = module.security.ecs_public_services_sg_id
  attach_to_alb         = true
  alb_target_group_arn  = module.alb.payments_service_target_group_arn

  environment_variables = {
    HTTP_PORT                    = "8080"
    DB_HOST                      = module.rds_mariadb.endpoint
    DB_PORT                      = tostring(module.rds_mariadb.port)
    DB_NAME                      = module.rds_mariadb.database_name
    DB_USER                      = module.rds_mariadb.master_username
    KAFKA_BROKERS                = module.msk.bootstrap_brokers_sasl_iam
    KAFKA_TOPIC_PAYMENTS_CREATED = "payments.created"
    OUTBOX_POLL_INTERVAL         = "500ms"
    OUTBOX_BATCH_SIZE            = "50"
    CORS_ALLOWED_ORIGIN          = "https://${module.frontend.cloudfront_domain_name}"
  }
  secrets = {
    DB_PASSWORD = module.rds_mariadb.master_password_secret_arn
  }
  secret_arns = [module.rds_mariadb.master_password_secret_arn]

  task_role_policy_json = data.aws_iam_policy_document.combined_task_policy.json

  enable_metrics_sidecar        = true
  otel_collector_config_ssm_arn = aws_ssm_parameter.otel_config_payments_service.arn

  tags = local.tags
}

module "payment_processor" {
  source = "../../modules/ecs-service"

  name_prefix        = local.name_prefix
  service_name       = "payment-processor"
  aws_region         = var.aws_region
  cluster_id         = module.ecs_cluster.cluster_id
  log_group_name     = module.ecs_cluster.log_group_name
  container_port     = 9091
  cpu                = 1024
  memory             = 2048
  desired_count      = 2
  subnet_ids         = module.network.private_subnet_ids
  security_group_id  = module.security.ecs_internal_services_sg_id
  attach_to_alb      = false

  environment_variables = {
    KAFKA_BROKERS                  = module.msk.bootstrap_brokers_sasl_iam
    KAFKA_CONSUMER_GROUP           = "payment-processor"
    KAFKA_TOPIC_PAYMENTS_CREATED   = "payments.created"
    KAFKA_TOPIC_PAYMENTS_PROCESSED = "payments.processed"
    KAFKA_TOPIC_PAYMENTS_DLQ       = "payments.dlq"
    WORKER_COUNT                   = "20" # prod: more headroom than the local default of 10
    WORKER_QUEUE_SIZE              = "200"
    PAYMENT_PROCESSING_TIMEOUT     = "5s"
    PAYMENT_MAX_RETRIES            = "3"
    PAYMENT_RETRY_BACKOFF          = "200ms"
    METRICS_PORT                   = "9091"
  }

  task_role_policy_json = data.aws_iam_policy_document.combined_task_policy.json

  enable_metrics_sidecar        = true
  otel_collector_config_ssm_arn = aws_ssm_parameter.otel_config_payment_processor.arn

  tags = local.tags
}

module "analytics_service" {
  source = "../../modules/ecs-service"

  name_prefix           = local.name_prefix
  service_name          = "analytics-service"
  aws_region            = var.aws_region
  cluster_id            = module.ecs_cluster.cluster_id
  log_group_name        = module.ecs_cluster.log_group_name
  container_port        = 8082
  cpu                   = 1024
  memory                = 2048
  desired_count         = 2
  subnet_ids            = module.network.private_subnet_ids
  security_group_id     = module.security.ecs_public_services_sg_id
  attach_to_alb         = true
  alb_target_group_arn  = module.alb.analytics_service_target_group_arn

  environment_variables = {
    HTTP_PORT                      = "8082"
    CLICKHOUSE_ADDR                = "${module.clickhouse.private_ip}:9000"
    CLICKHOUSE_DATABASE            = "analytics"
    CLICKHOUSE_USER                = "default"
    KAFKA_BROKERS                  = module.msk.bootstrap_brokers_sasl_iam
    KAFKA_CONSUMER_GROUP           = "analytics-service"
    KAFKA_TOPIC_PAYMENTS_PROCESSED = "payments.processed"
    KAFKA_TOPIC_PAYMENTS_DLQ       = "payments.dlq"
    ANALYTICS_BATCH_MAX_SIZE       = "500"
    ANALYTICS_BATCH_MAX_INTERVAL   = "2s"
    CORS_ALLOWED_ORIGIN            = "https://${module.frontend.cloudfront_domain_name}"
  }
  secrets = {
    CLICKHOUSE_PASSWORD = module.clickhouse.password_secret_arn
  }
  secret_arns = [module.clickhouse.password_secret_arn]

  task_role_policy_json = data.aws_iam_policy_document.combined_task_policy.json

  enable_metrics_sidecar        = true
  otel_collector_config_ssm_arn = aws_ssm_parameter.otel_config_analytics_service.arn

  tags = local.tags
}

# --- Frontend --------------------------------------------------------------

module "frontend" {
  source = "../../modules/frontend-static-site"

  name_prefix            = local.name_prefix
  bucket_name            = var.frontend_bucket_name
  domain_name            = var.frontend_domain_name
  acm_certificate_arn    = var.frontend_acm_certificate_arn
  cloudfront_price_class = "PriceClass_All" # prod: full global edge coverage, unlike dev/staging's PriceClass_100
  tags                   = local.tags
}
