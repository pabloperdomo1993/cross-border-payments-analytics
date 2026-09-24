variable "environment" {
  type    = string
  default = "staging"
}

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "vpc_cidr" {
  type    = string
  default = "10.1.0.0/16" # distinct from dev's 10.0.0.0/16 and prod's 10.2.0.0/16
}

variable "acm_certificate_arn" {
  description = "ACM certificate for the ALB's HTTPS listener, issued in aws_region. Dummy placeholder — replace with a real certificate ARN before applying."
  type        = string
  default     = "arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000"
}

variable "frontend_bucket_name" {
  description = "Must be globally unique across all of AWS."
  type        = string
  default     = "cbpa-staging-frontend-123456789012"
}

variable "frontend_domain_name" {
  type    = string
  default = null
}

variable "frontend_acm_certificate_arn" {
  description = "MUST be issued in us-east-1 regardless of aws_region (a CloudFront requirement). null when frontend_domain_name is also null."
  type        = string
  default     = null
}
