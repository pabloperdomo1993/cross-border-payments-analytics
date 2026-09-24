variable "aws_region" {
  description = "AWS region the state bucket/lock table live in."
  type        = string
  default     = "us-east-1"
}

variable "state_bucket_name" {
  description = "Globally-unique S3 bucket name for Terraform remote state. S3 bucket names are global across ALL AWS accounts, so the default below (with a dummy account id) WILL need to be changed to something actually unique before this can be applied."
  type        = string
  default     = "cbpa-terraform-state-123456789012" # dummy AWS account id — replace with your real one (or any other globally-unique suffix)
}

variable "lock_table_name" {
  description = "DynamoDB table name used for state locking."
  type        = string
  default     = "cbpa-terraform-locks"
}
