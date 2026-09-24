variable "name_prefix" {
  description = "Prefix applied to the cluster/log group name, e.g. \"cbpa-dev\"."
  type        = string
}

variable "enable_container_insights" {
  description = "Whether to enable CloudWatch Container Insights (extra per-task CPU/memory metrics, at extra cost). Recommended for staging/prod, optional for dev."
  type        = bool
  default     = false
}

variable "log_retention_days" {
  description = "CloudWatch log retention for all services in this cluster."
  type        = number
  default     = 14
}

variable "tags" {
  description = "Common tags merged into every resource this module creates."
  type        = map(string)
  default     = {}
}
