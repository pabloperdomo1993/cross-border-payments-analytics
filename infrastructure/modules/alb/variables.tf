variable "name_prefix" {
  description = "Prefix applied to every resource name, e.g. \"cbpa-dev\"."
  type        = string
}

variable "vpc_id" {
  type = string
}

variable "public_subnet_ids" {
  type = list(string)
}

variable "security_group_id" {
  description = "The ALB's own security group (see modules/security's alb_sg_id output)."
  type        = string
}

variable "acm_certificate_arn" {
  description = "ACM certificate ARN for the HTTPS listener. Must be issued in the SAME region as the ALB (unlike CloudFront, which requires us-east-1 regardless of region — see modules/frontend-static-site). Dummy example: \"arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000\"."
  type        = string
}

variable "payments_service_port" {
  type    = number
  default = 8080
}

variable "analytics_service_port" {
  type    = number
  default = 8082
}

variable "enable_deletion_protection" {
  description = "Recommended true for prod, false for dev/staging so the environment can be torn down freely."
  type        = bool
  default     = false
}

variable "tags" {
  description = "Common tags merged into every resource this module creates."
  type        = map(string)
  default     = {}
}
