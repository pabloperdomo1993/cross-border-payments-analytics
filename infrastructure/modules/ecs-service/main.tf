# One reusable module, instantiated once per Go service
# (payments-service, payment-processor, analytics-service — see
# environments/*/main.tf). Everything specific to one service (image,
# port, whether it sits behind the ALB, its env vars/secrets) comes in
# as a variable; nothing here hardcodes a particular service's identity.

resource "aws_ecr_repository" "this" {
  name                 = "${var.name_prefix}-${var.service_name}"
  image_tag_mutability = "IMMUTABLE" # a given tag (e.g. a git SHA) can never be silently overwritten

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-ecr" })
}

resource "aws_ecr_lifecycle_policy" "this" {
  repository = aws_ecr_repository.this.name
  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Keep only the most recent ${var.ecr_image_retention_count} images"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = var.ecr_image_retention_count
      }
      action = { type = "expire" }
    }]
  })
}

# --- IAM ---------------------------------------------------------------

data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Execution role: used by the ECS agent itself to pull the image, write
# logs, and fetch secret values to inject as environment variables at
# container start. This is deliberately NOT the same role the running
# application code uses (see task role below) — the execution role's
# permissions are needed before the application ever runs.
resource "aws_iam_role" "execution" {
  name               = "${var.name_prefix}-${var.service_name}-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "execution_managed" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Least-privilege addition on top of the AWS managed policy above: only
# the specific secret ARNs this service actually needs, never
# secretsmanager:* on every secret in the account.
resource "aws_iam_role_policy" "execution_secrets" {
  count = length(var.secret_arns) > 0 ? 1 : 0
  name  = "${var.name_prefix}-${var.service_name}-secrets-read"
  role  = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = var.secret_arns
    }]
  })
}

# Task role: what the RUNNING application (the Go binary itself)
# assumes — e.g. MSK IAM-auth permissions for produce/consume, or
# permission to push custom metrics. Empty by default; a service that
# needs nothing beyond what it gets from the network gets no extra
# permissions at all.
resource "aws_iam_role" "task" {
  name               = "${var.name_prefix}-${var.service_name}-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy" "task_inline" {
  count  = var.task_role_policy_json != null ? 1 : 0
  name   = "${var.name_prefix}-${var.service_name}-task-policy"
  role   = aws_iam_role.task.id
  policy = var.task_role_policy_json
}

# --- Task definition -----------------------------------------------------

locals {
  main_container = {
    name      = var.service_name
    image     = "${aws_ecr_repository.this.repository_url}:${var.image_tag}"
    essential = true
    portMappings = [{
      containerPort = var.container_port
      protocol      = "tcp"
    }]
    environment = [for k, v in var.environment_variables : { name = k, value = v }]
    secrets     = [for k, arn in var.secrets : { name = k, valueFrom = arn }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = var.log_group_name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = var.service_name
      }
    }
  }

  # Optional ADOT (AWS Distro for OpenTelemetry) sidecar: scrapes this
  # service's own /metrics endpoint and remote-writes to Amazon Managed
  # Prometheus, since AMP has no built-in way to pull metrics from an
  # arbitrary private ECS task the way local Prometheus scrapes
  # localhost directly (see modules/observability). Reads its collector
  # config from an SSM parameter rather than embedding YAML in the task
  # definition, so the scrape/remote-write config can change without a
  # new task definition revision.
  metrics_sidecar = var.enable_metrics_sidecar ? [{
    name      = "${var.service_name}-otel-collector"
    image     = "public.ecr.aws/aws-observability/aws-otel-collector:latest"
    essential = false
    command   = ["--config=env:AOT_CONFIG_CONTENT"]
    secrets = [{
      name      = "AOT_CONFIG_CONTENT"
      valueFrom = var.otel_collector_config_ssm_arn
    }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = var.log_group_name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "${var.service_name}-otel"
      }
    }
  }] : []

  container_definitions = concat([local.main_container], local.metrics_sidecar)
}

resource "aws_ecs_task_definition" "this" {
  family                   = "${var.name_prefix}-${var.service_name}"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.cpu
  memory                   = var.memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn
  container_definitions    = jsonencode(local.container_definitions)

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-task" })
}

# --- Service -------------------------------------------------------------

resource "aws_ecs_service" "this" {
  name            = "${var.name_prefix}-${var.service_name}"
  cluster         = var.cluster_id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets         = var.subnet_ids
    security_groups = [var.security_group_id]
    # Tasks live in private subnets; they reach ECR/Secrets
    # Manager/CloudWatch/MSK/AMP through the environment's NAT gateway
    # (or VPC endpoints, a documented future optimization — see
    # infrastructure/README.md), never by having a public IP themselves.
    assign_public_ip = false
  }

  dynamic "load_balancer" {
    for_each = var.attach_to_alb ? [1] : []
    content {
      target_group_arn = var.alb_target_group_arn
      container_name   = var.service_name
      container_port   = var.container_port
    }
  }

  # Only meaningful for ALB-attached services — an internal service like
  # payment-processor has no load balancer health check to wait on.
  health_check_grace_period_seconds = var.attach_to_alb ? 60 : null

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-service" })

  lifecycle {
    # The image tag deployed is meant to be updated out-of-band (by a
    # CI/CD pipeline pushing a new task definition revision), not by
    # re-running `terraform apply` with a stale image_tag default.
    ignore_changes = [task_definition]
  }
}
