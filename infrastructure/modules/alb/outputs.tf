output "dns_name" {
  value = aws_lb.this.dns_name
}

output "payments_service_target_group_arn" {
  value = aws_lb_target_group.payments_service.arn
}

output "analytics_service_target_group_arn" {
  value = aws_lb_target_group.analytics_service.arn
}
