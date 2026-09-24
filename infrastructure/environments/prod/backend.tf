# Same bucket/table as dev/staging (see ../../bootstrap), different
# state key.
terraform {
  backend "s3" {
    bucket         = "cbpa-terraform-state-123456789012"
    key            = "prod/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "cbpa-terraform-locks"
    encrypt        = true
  }
}
