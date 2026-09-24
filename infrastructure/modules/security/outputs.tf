output "alb_sg_id" {
  value = aws_security_group.alb.id
}

output "ecs_public_services_sg_id" {
  description = "Attach to payments-service and analytics-service (the two ALB-fronted services)."
  value       = aws_security_group.ecs_public_services.id
}

output "ecs_internal_services_sg_id" {
  description = "Attach to payment-processor (no public HTTP API, never attached to the ALB)."
  value       = aws_security_group.ecs_internal_services.id
}

output "rds_sg_id" {
  value = aws_security_group.rds.id
}

output "clickhouse_sg_id" {
  value = aws_security_group.clickhouse.id
}

output "msk_sg_id" {
  value = aws_security_group.msk.id
}
