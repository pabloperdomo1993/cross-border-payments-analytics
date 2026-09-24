variable "name_prefix" {
  type = string
}

variable "bucket_name" {
  description = "Globally-unique S3 bucket name for the built frontend assets. Dummy example: \"cbpa-dev-frontend-123456789012\"."
  type        = string
}

variable "domain_name" {
  description = "Custom domain to serve the frontend from, e.g. \"app.example.com\". null uses only CloudFront's own *.cloudfront.net domain."
  type        = string
  default     = null
}

variable "acm_certificate_arn" {
  description = "ACM certificate ARN for domain_name, MUST be issued in us-east-1 regardless of this stack's primary region (a hard CloudFront requirement). null when domain_name is also null. Dummy example: \"arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000\"."
  type        = string
  default     = null
}

variable "cloudfront_price_class" {
  description = "PriceClass_100 (North America + Europe only, cheapest) is enough for dev/staging; consider PriceClass_All for prod if global latency matters."
  type        = string
  default     = "PriceClass_100"
}

variable "tags" {
  type    = map(string)
  default = {}
}
