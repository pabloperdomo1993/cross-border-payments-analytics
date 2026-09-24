variable "name_prefix" {
  type = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "security_group_id" {
  description = "See modules/security's rds_sg_id output."
  type        = string
}

variable "database_name" {
  type    = string
  default = "payments" # matches docker-compose.yml's MARIADB_DATABASE
}

variable "master_username" {
  type    = string
  default = "payments" # matches docker-compose.yml's MARIADB_USER
}

variable "engine_version" {
  description = "MariaDB engine version. Left unpinned to a specific patch version by default so RDS applies minor patches; pin explicitly for prod if strict version control is required."
  type        = string
  default     = "11.4"
}

variable "parameter_group_family" {
  type    = string
  default = "mariadb11.4"
}

variable "instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "allocated_storage_gb" {
  type    = number
  default = 20
}

variable "max_allocated_storage_gb" {
  description = "Storage autoscaling ceiling. Should be comfortably above allocated_storage_gb."
  type        = number
  default     = 100
}

variable "multi_az" {
  description = "true for prod (automatic standby failover); false for dev/staging."
  type        = bool
  default     = false
}

variable "backup_retention_days" {
  type    = number
  default = 1
}

variable "is_production" {
  description = "Gates deletion protection and final-snapshot behavior. Set true only for the prod environment."
  type        = bool
  default     = false
}

variable "tags" {
  type    = map(string)
  default = {}
}
