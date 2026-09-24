output "amp_workspace_id" {
  value = aws_prometheus_workspace.this.id
}

output "amp_workspace_arn" {
  value = aws_prometheus_workspace.this.arn
}

output "amp_remote_write_endpoint" {
  description = "Pass to modules/ecs-service's ADOT sidecar collector config as the remote_write target."
  value       = "${aws_prometheus_workspace.this.prometheus_endpoint}api/v1/remote_write"
}

output "amg_workspace_id" {
  value = aws_grafana_workspace.this.id
}

output "amg_endpoint" {
  value = aws_grafana_workspace.this.endpoint
}
