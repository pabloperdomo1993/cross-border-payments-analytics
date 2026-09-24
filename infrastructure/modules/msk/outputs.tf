output "cluster_arn" {
  value = aws_msk_serverless_cluster.this.arn
}

output "bootstrap_brokers_sasl_iam" {
  value = aws_msk_serverless_cluster.this.bootstrap_brokers_sasl_iam
}
