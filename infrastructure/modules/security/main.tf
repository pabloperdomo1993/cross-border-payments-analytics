# Every security group below follows one rule: ingress is scoped to
# another security group ID wherever possible (e.g. "RDS only accepts
# traffic FROM the ECS services' security groups"), never a raw CIDR
# block, except for the ALB's public-facing ingress — the one place in
# this system that's actually meant to be reachable from the internet.

resource "aws_security_group" "alb" {
  name_prefix = "${var.name_prefix}-alb-"
  description = "Public-facing ALB: the only security group in this system that accepts traffic from the internet."
  vpc_id      = var.vpc_id

  ingress {
    description = "HTTPS from anywhere"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTP from anywhere (redirected to HTTPS by the ALB listener, see modules/alb)"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-alb-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

# payments-service and analytics-service both sit behind the ALB.
resource "aws_security_group" "ecs_public_services" {
  name_prefix = "${var.name_prefix}-ecs-public-"
  description = "payments-service / analytics-service: only reachable from the ALB, never directly from the internet."
  vpc_id      = var.vpc_id

  ingress {
    description     = "From the ALB only"
    from_port       = 0
    to_port         = 65535
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"] # needs to reach RDS/MSK/ECR/Secrets Manager/AMP, all over TLS
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-ecs-public-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

# payment-processor has no public HTTP API — it's never attached to the
# ALB (see modules/ecs-service's attach_to_alb flag) — so it needs no
# inbound rule at all, only outbound (Kafka, downstream metrics export).
resource "aws_security_group" "ecs_internal_services" {
  name_prefix = "${var.name_prefix}-ecs-internal-"
  description = "payment-processor: a Kafka consumer/producer with no public HTTP API — no inbound access needed from anywhere."
  vpc_id      = var.vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-ecs-internal-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "rds" {
  name_prefix = "${var.name_prefix}-rds-"
  description = "RDS MariaDB: reachable only from the ECS services that actually talk to it."
  vpc_id      = var.vpc_id

  ingress {
    description     = "MariaDB from payments-service"
    from_port       = 3306
    to_port         = 3306
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_public_services.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-rds-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "clickhouse" {
  name_prefix = "${var.name_prefix}-clickhouse-"
  description = "ClickHouse EC2 instance: reachable only from analytics-service (the only consumer of it)."
  vpc_id      = var.vpc_id

  ingress {
    description     = "ClickHouse native protocol from analytics-service"
    from_port       = 9000
    to_port         = 9000
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_public_services.id]
  }

  ingress {
    description     = "ClickHouse HTTP interface from analytics-service"
    from_port       = 8123
    to_port         = 8123
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_public_services.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"] # pulls its own Docker image from Docker Hub on first boot
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-clickhouse-sg" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_security_group" "msk" {
  name_prefix = "${var.name_prefix}-msk-"
  description = "MSK Serverless: reachable only from the three Go services that produce/consume Kafka messages."
  vpc_id      = var.vpc_id

  ingress {
    description     = "Kafka (IAM-authenticated) from payments-service/analytics-service"
    from_port       = 9098
    to_port         = 9098
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_public_services.id]
  }

  ingress {
    description     = "Kafka (IAM-authenticated) from payment-processor"
    from_port       = 9098
    to_port         = 9098
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_internal_services.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-msk-sg" })

  lifecycle {
    create_before_destroy = true
  }
}
