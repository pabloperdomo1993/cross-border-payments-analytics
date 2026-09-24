variable "name_prefix" {
  description = "Prefix applied to every resource name/tag this module creates, e.g. \"cbpa-dev\"."
  type        = string
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC. Each environment should use a distinct, non-overlapping range (dev/staging/prod are never peered here, but distinct ranges avoid future surprises)."
  type        = string
}

variable "single_nat_gateway" {
  description = "true = one shared NAT gateway (cheaper, used for dev/staging). false = one NAT gateway per AZ (higher availability, used for prod)."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Common tags merged into every resource this module creates."
  type        = map(string)
  default     = {}
}
