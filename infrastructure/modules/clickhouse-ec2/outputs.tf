output "private_ip" {
  value = aws_instance.this.private_ip
}

output "instance_id" {
  value = aws_instance.this.id
}

output "password_secret_arn" {
  value = aws_secretsmanager_secret.clickhouse_password.arn
}
