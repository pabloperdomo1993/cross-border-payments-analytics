# One Fargate cluster per environment. Fargate over EKS/self-managed
# EC2: three small services don't justify running (and patching) a
# Kubernetes control plane or a fleet of EC2 instances — Fargate removes
# the underlying host entirely, which is the right trade for this scale.

resource "aws_ecs_cluster" "this" {
  name = "${var.name_prefix}-cluster"

  setting {
    name  = "containerInsights"
    value = var.enable_container_insights ? "enabled" : "disabled"
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-cluster" })
}

# Fargate has no concept of "the cluster's own capacity" the way EC2
# does, but it still needs a default capacity provider strategy so
# `aws_ecs_service` resources don't have to repeat this everywhere.
resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE"
    weight            = 100
  }
}

# One shared log group; each service's ecs-service module writes to its
# own stream prefix within it, so logs stay in one place per environment
# without needing a log group per service.
resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name_prefix}"
  retention_in_days = var.log_retention_days

  tags = merge(var.tags, { Name = "${var.name_prefix}-ecs-logs" })
}
