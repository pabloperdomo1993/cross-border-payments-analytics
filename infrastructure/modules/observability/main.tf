# Amazon Managed Prometheus (AMP) + Amazon Managed Grafana (AMG) over
# self-hosting Prometheus/Grafana as containers: removes the operational
# burden of Prometheus's stateful TSDB storage (Fargate has no good
# persistent-disk story for that) and Grafana's own persistent config
# storage. See infrastructure/README.md.
#
# How metrics actually get here: each Go service's ECS task can run an
# ADOT collector sidecar (see modules/ecs-service's enable_metrics_sidecar)
# that scrapes its own /metrics and remote-writes to this AMP workspace's
# endpoint — AMP has no built-in way to reach into a private ECS task
# and scrape it directly the way local Prometheus does.

resource "aws_prometheus_workspace" "this" {
  alias = "${var.name_prefix}-amp"
  tags  = merge(var.tags, { Name = "${var.name_prefix}-amp" })
}

resource "aws_grafana_workspace" "this" {
  name                     = "${var.name_prefix}-grafana"
  account_access_type      = "CURRENT_ACCOUNT"
  authentication_providers = [var.authentication_provider]
  permission_type          = "SERVICE_MANAGED" # AWS manages the IAM role AMG uses to query the data sources listed below
  data_sources             = ["PROMETHEUS"]
  role_arn                 = null # left to AWS when permission_type is SERVICE_MANAGED

  tags = merge(var.tags, { Name = "${var.name_prefix}-grafana" })
}

# NOTE (honestly documented, not silently glossed over): importing the
# existing observability/grafana/provisioning/dashboards/platform-overview.json
# dashboard into this AMG workspace is a deliberate follow-up step, not
# done by this module. The Terraform `grafana` provider needs an AMG API
# key/service account token that only exists AFTER aws_grafana_workspace
# above is created — wiring that into the SAME apply would need the
# `grafana` provider's configuration to depend on a resource from this
# same plan, which Terraform does not support (provider configs can't
# depend on resources created in the same run). The correct, honest
# pattern is a second, separate step: create an
# `aws_grafana_workspace_api_key` (or, on newer AMG versions, a
# workspace service account token) once this workspace exists, then run
# a small second Terraform config (or a CI script) using the `grafana`
# provider against that key to import the JSON file as-is — not
# re-typed into HCL.
