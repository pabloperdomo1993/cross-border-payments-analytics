variable "name_prefix" {
  type = string
}

variable "authentication_provider" {
  description = "\"AWS_SSO\" requires IAM Identity Center enabled in the account; \"SAML\" is the alternative if using an external identity provider instead. Defaults to AWS_SSO as the simpler AWS-native option."
  type        = string
  default     = "AWS_SSO"
}

variable "tags" {
  type    = map(string)
  default = {}
}
