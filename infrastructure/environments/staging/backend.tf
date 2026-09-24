# Same bucket/table as dev (see ../../bootstrap), different state key —
# one bucket holds every environment's state, distinguished by key.
terraform {
  backend "s3" {
    bucket         = "cbpa-terraform-state-123456789012"
    key            = "staging/terraform.tfstate"
    region         = "us-east-1"
    dynamodb_table = "cbpa-terraform-locks"
    encrypt        = true
  }
}
