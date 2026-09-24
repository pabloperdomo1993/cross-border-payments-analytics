# Bootstraps the S3 bucket + DynamoDB table that every environment's
# Terraform state (dev/staging/prod, see ../environments/*) is stored
# in and locked with.
#
# This config deliberately does NOT use a remote backend itself — you
# can't store Terraform's own state in a bucket that doesn't exist yet.
# It runs once, with plain local state, by whoever first sets this
# project up on AWS. After it applies successfully, the resulting
# bucket/table names go into each environment's backend.tf and this
# directory is never touched again (its own local .tfstate can be kept
# somewhere safe, or re-imported later if it's ever lost — the bucket
# and table it created are not, by themselves, destroyed by that).
terraform {
  required_version = ">= 1.9"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# One state bucket and one lock table for ALL THREE environments —
# each environment gets its own object key (see environments/*/backend.tf),
# so a single bucket/table pair is enough; there is no need for one per
# environment, only one per AWS account/project.
resource "aws_s3_bucket" "terraform_state" {
  bucket = var.state_bucket_name

  # Buckets holding Terraform state are exactly the kind of resource you
  # never want deleted by an accidental `terraform destroy` run from the
  # wrong directory.
  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Project   = "cross-border-payments-analytics"
    ManagedBy = "terraform"
    Purpose   = "terraform-state"
  }
}

resource "aws_s3_bucket_versioning" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  versioning_configuration {
    # Versioning turns a bad `apply` that corrupts state into a
    # recoverable mistake instead of a permanent one.
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_dynamodb_table" "terraform_lock" {
  name         = var.lock_table_name
  billing_mode = "PAY_PER_REQUEST" # no fixed capacity to size for a table this small/infrequent
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Project   = "cross-border-payments-analytics"
    ManagedBy = "terraform"
    Purpose   = "terraform-state-lock"
  }
}
