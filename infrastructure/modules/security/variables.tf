variable "name_prefix" {
  description = "Prefix applied to every security group name/tag, e.g. \"cbpa-dev\"."
  type        = string
}

variable "vpc_id" {
  description = "VPC these security groups belong to (see modules/network)."
  type        = string
}

variable "tags" {
  description = "Common tags merged into every resource this module creates."
  type        = map(string)
  default     = {}
}
