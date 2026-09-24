# Managed MariaDB, matching the local docker-compose `mariadb:11`
# service exactly (same engine, same database/user name defaults) — see
# infrastructure/README.md for why RDS over self-managing MariaDB on
# EC2 (the same "let AWS manage it" reasoning as MSK/RDS elsewhere).

resource "random_password" "master" {
  length  = 32
  special = false # some MariaDB client libraries mishandle certain special characters in DSNs; digits+letters is plenty of entropy at this length
}

resource "aws_secretsmanager_secret" "master_password" {
  name = "${var.name_prefix}/mariadb/master-password"
  tags = var.tags
}

resource "aws_secretsmanager_secret_version" "master_password" {
  secret_id     = aws_secretsmanager_secret.master_password.id
  secret_string = random_password.master.result
}

resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-mariadb"
  subnet_ids = var.private_subnet_ids
  tags       = merge(var.tags, { Name = "${var.name_prefix}-mariadb-subnet-group" })
}

resource "aws_db_parameter_group" "this" {
  name   = "${var.name_prefix}-mariadb"
  family = var.parameter_group_family

  tags = var.tags
}

resource "aws_db_instance" "this" {
  identifier     = "${var.name_prefix}-mariadb"
  engine         = "mariadb"
  engine_version = var.engine_version
  instance_class = var.instance_class

  allocated_storage     = var.allocated_storage_gb
  max_allocated_storage = var.max_allocated_storage_gb # enables storage autoscaling instead of a hard ceiling
  storage_type          = "gp3"
  storage_encrypted     = true

  db_name  = var.database_name
  username = var.master_username
  password = random_password.master.result

  db_subnet_group_name   = aws_db_subnet_group.this.name
  parameter_group_name   = aws_db_parameter_group.this.name
  vpc_security_group_ids = [var.security_group_id]
  publicly_accessible    = false

  multi_az = var.multi_az

  backup_retention_period = var.backup_retention_days
  backup_window           = "03:00-04:00" # low-traffic window, arbitrary but consistent
  maintenance_window      = "mon:04:30-mon:05:30"

  # Prod keeps a final snapshot on destroy; dev/staging skip it so
  # tearing down a throwaway environment doesn't leave billed snapshots
  # behind indefinitely.
  skip_final_snapshot       = !var.is_production
  final_snapshot_identifier = var.is_production ? "${var.name_prefix}-mariadb-final" : null
  deletion_protection       = var.is_production

  tags = merge(var.tags, { Name = "${var.name_prefix}-mariadb" })
}
