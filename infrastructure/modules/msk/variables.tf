variable "name_prefix" {
  type = string
}

variable "private_subnet_ids" {
  description = "MSK Serverless requires at least 2 subnets in different AZs."
  type        = list(string)
}

variable "security_group_id" {
  description = "See modules/security's msk_sg_id output."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
