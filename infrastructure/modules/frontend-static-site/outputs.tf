output "bucket_name" {
  value = aws_s3_bucket.site.id
}

output "cloudfront_domain_name" {
  value = aws_cloudfront_distribution.site.domain_name
}

output "cloudfront_distribution_id" {
  description = "Needed by CI/CD to invalidate the cache after deploying a new build."
  value       = aws_cloudfront_distribution.site.id
}
