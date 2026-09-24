output "alb_dns_name" {
  value = module.alb.dns_name
}

output "frontend_cloudfront_domain_name" {
  value = module.frontend.cloudfront_domain_name
}

output "frontend_bucket_name" {
  value = module.frontend.bucket_name
}

output "rds_endpoint" {
  value = module.rds_mariadb.endpoint
}

output "clickhouse_private_ip" {
  value = module.clickhouse.private_ip
}

output "kafka_bootstrap_brokers" {
  value = module.msk.bootstrap_brokers_sasl_iam
}

output "amg_endpoint" {
  value = module.observability.amg_endpoint
}

output "ecr_repository_urls" {
  value = {
    payments-service  = module.payments_service.ecr_repository_url
    payment-processor = module.payment_processor.ecr_repository_url
    analytics-service = module.analytics_service.ecr_repository_url
  }
}
