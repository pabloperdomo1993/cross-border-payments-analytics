# MSK Serverless over self-managed Kafka on EC2 or provisioned MSK: no
# broker count/instance sizing to choose, storage and throughput scale
# automatically, and IAM-based authentication replaces local's PLAINTEXT
# listener with real access control. See infrastructure/README.md.
#
# Topic creation (payments.created / payments.processed / payments.dlq,
# 6 partitions each — matching kafka-init in docker-compose.yml) is NOT
# done here: MSK Serverless has no Terraform-manageable topic resource
# the way self-managed Kafka does, and auto.create.topics is disabled by
# design. Topics are created once, out-of-band, using the Kafka CLI
# against this cluster's bootstrap brokers (documented in
# infrastructure/README.md) — the same kind of one-time operational step
# kafka-init already performs locally, just not expressible as a
# Terraform resource for MSK Serverless.

resource "aws_msk_serverless_cluster" "this" {
  cluster_name = "${var.name_prefix}-kafka"

  vpc_config {
    subnet_ids         = var.private_subnet_ids
    security_group_ids = [var.security_group_id]
  }

  client_authentication {
    sasl {
      iam {
        enabled = true
      }
    }
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-kafka" })
}
