variable "name_prefix" {
  description = "Prefix applied to every resource name, e.g. \"cbpa-dev\"."
  type        = string
}

variable "service_name" {
  description = "The service's own name, e.g. \"payments-service\". Used as the container name, ECR repo suffix, and CloudWatch log stream prefix."
  type        = string
}

variable "aws_region" {
  description = "Region this service runs in (needed for the awslogs log driver configuration)."
  type        = string
}

variable "cluster_id" {
  description = "ECS cluster ID this service is deployed into (see modules/ecs-cluster)."
  type        = string
}

variable "log_group_name" {
  description = "CloudWatch log group this service's containers write to (see modules/ecs-cluster)."
  type        = string
}

variable "image_tag" {
  description = "Image tag to deploy on first create. Ignored on subsequent applies (see the task definition's lifecycle.ignore_changes) — real deployments are expected to happen via CI/CD registering a new task definition revision, not via terraform apply."
  type        = string
  default     = "latest"
}

variable "container_port" {
  description = "Port the service's container listens on, e.g. 8080."
  type        = number
}

variable "cpu" {
  description = "Fargate task CPU units (256 = 0.25 vCPU, 1024 = 1 vCPU, ...)."
  type        = number
  default     = 256
}

variable "memory" {
  description = "Fargate task memory, in MiB."
  type        = number
  default     = 512
}

variable "desired_count" {
  description = "Number of task instances to run."
  type        = number
  default     = 1
}

variable "environment_variables" {
  description = "Plain (non-secret) environment variables for the main container."
  type        = map(string)
  default     = {}
}

variable "secrets" {
  description = "Environment variables whose value is injected from Secrets Manager at container start. Map of env var name -> secret ARN (or ARN:jsonKey for a specific key within a JSON secret)."
  type        = map(string)
  default     = {}
}

variable "secret_arns" {
  description = "The distinct Secrets Manager ARNs referenced in `secrets` above, granted to the execution role via secretsmanager:GetSecretValue. Kept as an explicit list (rather than derived from `secrets`' values) so a caller can grant read access to a secret ARN prefix/wildcard-free base ARN even when `secrets` references a JSON key suffix."
  type        = list(string)
  default     = []
}

variable "task_role_policy_json" {
  description = "Optional inline IAM policy (JSON) granted to the task role — e.g. MSK IAM-auth permissions. null means the task role gets no extra permissions beyond the ability to be assumed."
  type        = string
  default     = null
}

variable "subnet_ids" {
  description = "Private subnet IDs the service's tasks run in."
  type        = list(string)
}

variable "security_group_id" {
  description = "Security group attached to this service's tasks."
  type        = string
}

variable "attach_to_alb" {
  description = "Whether this service is registered with an ALB target group (true for payments-service/analytics-service; false for payment-processor, which has no public HTTP API)."
  type        = bool
  default     = false
}

variable "alb_target_group_arn" {
  description = "ALB target group ARN to register with. Required when attach_to_alb is true."
  type        = string
  default     = null
}

variable "enable_metrics_sidecar" {
  description = "Whether to run an ADOT collector sidecar that scrapes this service's /metrics and remote-writes to Amazon Managed Prometheus."
  type        = bool
  default     = false
}

variable "otel_collector_config_ssm_arn" {
  description = "SSM Parameter ARN holding the ADOT collector's YAML config. Required when enable_metrics_sidecar is true."
  type        = string
  default     = null
}

variable "ecr_image_retention_count" {
  description = "How many most-recent images to keep in this service's ECR repo before older ones are expired."
  type        = number
  default     = 10
}

variable "tags" {
  description = "Common tags merged into every resource this module creates."
  type        = map(string)
  default     = {}
}
