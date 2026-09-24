variable "name_prefix" {
  type = string
}

variable "private_subnet_id" {
  description = "Single private subnet this instance runs in (no HA across AZs for ClickHouse at this project's current scale — see infrastructure/README.md)."
  type        = string
}

variable "availability_zone" {
  description = "Must match the AZ of private_subnet_id (EBS volumes are zonal)."
  type        = string
}

variable "security_group_id" {
  description = "See modules/security's clickhouse_sg_id output."
  type        = string
}

variable "instance_type" {
  type    = string
  default = "t3.medium"
}

variable "data_volume_size_gb" {
  type    = number
  default = 50
}

variable "database_name" {
  type    = string
  default = "analytics" # matches docker-compose.yml's CLICKHOUSE_DB
}

variable "tags" {
  type    = map(string)
  default = {}
}
