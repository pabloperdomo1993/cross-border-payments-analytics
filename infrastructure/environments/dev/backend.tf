# Points at the bucket/table created once by ../../bootstrap. Terraform
# does not allow variables here (the backend config must be static), so
# these values are the actual bootstrap output names, not placeholders
# to fill in per-machine — but they DO still contain a dummy AWS account
# id (see bootstrap/variables.tf) that needs replacing with a real,
# globally-unique bucket name before this can ever be applied.
terraform {
  backend "s3" {
    bucket         = "cbpa-terraform-state-123456789012"
    key            = "dev/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "cbpa-terraform-locks"
    encrypt        = true
  }
}
