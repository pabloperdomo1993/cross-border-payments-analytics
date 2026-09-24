# AWS has no managed ClickHouse offering (unlike RDS for MariaDB), so
# this runs the exact same clickhouse/clickhouse-server:24.8 image used
# locally on a single EC2 instance — reusing the tested image/config
# rather than inventing a new install path. See infrastructure/README.md
# for the single-node trade-off this accepts (matches the local setup;
# a production ClickHouse cluster with real HA is a larger, separate
# undertaking not justified at this project's current scale).

data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}

resource "random_password" "clickhouse" {
  length  = 32
  special = false
}

resource "aws_secretsmanager_secret" "clickhouse_password" {
  name = "${var.name_prefix}/clickhouse/password"
  tags = var.tags
}

resource "aws_secretsmanager_secret_version" "clickhouse_password" {
  secret_id     = aws_secretsmanager_secret.clickhouse_password.id
  secret_string = random_password.clickhouse.result
}

# Lets the instance be reached via SSM Session Manager instead of SSH —
# no open port 22, no key pair to distribute/rotate.
data "aws_iam_policy_document" "ec2_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "this" {
  name               = "${var.name_prefix}-clickhouse"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume_role.json
  tags               = var.tags
}

resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.this.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "this" {
  name = "${var.name_prefix}-clickhouse"
  role = aws_iam_role.this.name
}

resource "aws_ebs_volume" "data" {
  availability_zone = var.availability_zone
  size              = var.data_volume_size_gb
  type              = "gp3"
  encrypted         = true

  tags = merge(var.tags, { Name = "${var.name_prefix}-clickhouse-data" })
}

resource "aws_instance" "this" {
  ami                    = data.aws_ami.al2023.id
  instance_type          = var.instance_type
  subnet_id              = var.private_subnet_id
  vpc_security_group_ids = [var.security_group_id]
  iam_instance_profile   = aws_iam_instance_profile.this.name

  # Installs Docker and runs the same image used locally, with its data
  # directory bind-mounted onto the separate EBS volume attached below
  # (mounted at /mnt/clickhouse-data by the same script) rather than the
  # root volume, so the data survives an instance replacement as long as
  # the EBS volume is reattached.
  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    clickhouse_password = random_password.clickhouse.result
    database_name       = var.database_name
  })

  tags = merge(var.tags, { Name = "${var.name_prefix}-clickhouse" })

  lifecycle {
    # The password is baked into user_data at instance creation; a
    # future password rotation is a deliberate, separate operation
    # (replace the secret + reboot with new user_data), not something
    # `terraform apply` should do implicitly by replacing the instance.
    ignore_changes = [user_data]
  }
}

resource "aws_volume_attachment" "data" {
  device_name = "/dev/xvdf"
  volume_id   = aws_ebs_volume.data.id
  instance_id = aws_instance.this.id
}
