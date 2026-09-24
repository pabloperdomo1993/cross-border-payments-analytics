# Infrastructure — AWS deployment (Terraform)

This directory holds a complete, real-AWS deployment story for the
platform documented in [../docs/architecture.md](../docs/architecture.md),
expressed as Terraform. It exists **alongside**, not instead of, the
project's local Docker Compose setup (see the root
[README.md](../README.md)) — running the platform locally still needs
nothing but Docker.

> **This code has not been applied or validated against a real AWS
> account.** There is no AWS account behind this repository. Every
> value that would normally be account-specific (account IDs, domain
> names, certificate ARNs) is a clearly-marked dummy placeholder. Treat
> this as a complete, carefully-authored design — read it, review it,
> adapt the placeholders — not as something proven to `terraform apply`
> cleanly on the first try, the way the rest of this repo's own code is
> backed by passing tests and a running local stack.

## Architecture decisions

Each decision below follows the same Context → Options → Decision →
Trade-offs shape as the project's existing ADRs under
[../docs/decisions/](../docs/decisions/) (this file, not that directory,
per this task's own scope: everything new here lives under
`infrastructure/`).

### Compute for the three Go services → ECS Fargate

**Context**: `payments-service`, `payment-processor`, and
`analytics-service` are three small, independently deployable Go
binaries, each already a Docker image (see each service's `Dockerfile`).

**Options**:
- **EKS (Kubernetes)** — Advantages: most powerful/flexible orchestration, an ecosystem of tooling. Disadvantages: a control plane to pay for and reason about, real operational overhead (upgrades, node management even with managed node groups) for a system this small; nothing in this project's actual tech stack mentions Kubernetes.
- **Plain EC2 + Docker Compose** — Advantages: closest match to the local setup, cheapest at very small scale. Disadvantages: manual instance patching, no built-in rolling deploys or self-healing, scaling means scripting your own instance management.
- **ECS Fargate** — Advantages: no hosts to patch or size, task-level scaling, straightforward rolling deploys via ECS service updates, pay only for what a task actually uses. Disadvantages: less low-level control than EC2 (no SSH into the host itself), a per-vCPU/memory price premium over raw EC2.

**Decision**: ECS Fargate. Three services at this scale don't justify Kubernetes' operational floor, and Fargate removes exactly the kind of host-patching toil plain EC2 would otherwise add, without giving up rolling deploys or per-service scaling.

**Trade-offs**: accepting Fargate's per-resource cost premium and reduced host-level control in exchange for zero patching burden and simple, declarative scaling (`desired_count` per environment — see `environments/*/main.tf`).

`payment-processor` specifically is **not** attached to the ALB (see `modules/ecs-service`'s `attach_to_alb` flag) — it has no public HTTP API, only `/health`, `/ready`, `/metrics` for internal use, matching how it's already deployed locally.

### Frontend → S3 + CloudFront

**Context**: the frontend build (`npm run build`) produces a static SPA — HTML/CSS/JS with no server-side logic — currently served locally by a containerized nginx (see `frontend/Dockerfile`).

**Options**:
- **Containerized nginx on ECS Fargate** (mirrors local exactly) — Advantages: identical mental model to every other service, one deployment pattern for everything. Disadvantages: pays for an always-on task to serve files that never change between requests; no CDN edge caching without adding CloudFront in front of it anyway.
- **S3 + CloudFront** — Advantages: no compute to run at all, CloudFront's edge network caches assets close to users, S3 storage is pennies. Disadvantages: a second deployment mechanism (sync to S3 + CloudFront invalidation) distinct from the ECS services' image-push-and-redeploy flow.

**Decision**: S3 + CloudFront (`modules/frontend-static-site`) — the standard, cost-efficient AWS pattern for a pure static SPA. `custom_error_response` maps both 403 (private-bucket default for an unmatched key) and 404 to `index.html` with a 200, so client-side routing (React Router) works exactly like `nginx.conf`'s local `try_files $uri $uri/ /index.html`.

**Trade-offs**: a second, different deployment path from the ECS services (upload + cache invalidation, not a task definition update) — accepted because a static site genuinely doesn't need a container's lifecycle at all.

### MariaDB → Amazon RDS for MariaDB

**Context**: `payments-service` needs the operational, transactional database described in [ADR-002](../docs/decisions/002-database-strategy.md) — the same rationale for MariaDB itself applies unchanged; this decision is only about *who runs it*.

**Options**: self-managed MariaDB on EC2 (full control, full patching/backup burden) vs. **RDS for MariaDB** (managed patching, automated backups, Multi-AZ failover available, engine-compatible with the local `mariadb:11` image).

**Decision**: RDS (`modules/rds-mariadb`). Multi-AZ is enabled only in `prod` (`multi_az = true`) — dev/staging accept a single instance since their downtime tolerance is higher and Multi-AZ roughly doubles the instance cost.

**Trade-offs**: RDS costs more than a bare EC2 instance running the same engine, in exchange for AWS handling patching, backups, and (in prod) automatic failover — the same "let AWS manage it" trade this project already made for Kafka (see below).

### ClickHouse → self-managed on EC2 (no AWS-managed alternative exists)

**Context**: unlike MariaDB, **AWS has no managed ClickHouse service** — there is no "RDS for ClickHouse." The only options are self-hosting it (on EC2, ECS, or Kubernetes) or a third-party SaaS (e.g. ClickHouse Cloud), which introduces a vendor and billing relationship entirely outside AWS.

**Decision**: a single EC2 instance per environment (`modules/clickhouse-ec2`), running the *exact same* `clickhouse/clickhouse-server:24.8` Docker image the local `docker-compose.yml` already uses, with its data directory on a separately-attached, persistent EBS volume rather than the instance's root disk. This reuses a tested image/config instead of inventing a new install path, and accepts the same single-node trade-off the local setup already makes (see [ADR-002](../docs/decisions/002-database-strategy.md)) — a real multi-node ClickHouse cluster with replication is a substantially larger undertaking not justified at this project's current scale.

**Honestly documented gap**: this module does not apply `backend/analytics-service/migrations/*.sql` the way local Docker Compose does via `/docker-entrypoint-initdb.d` — those files live in this repository, not on the EC2 instance. Applying them here is a deliberate, separate follow-up (e.g. a small CI step running the same `.sql` files against the instance's ClickHouse HTTP interface once it's reachable), not silently assumed to happen.

### Kafka → MSK Serverless

**Context**: the local `kafka` service is a single-node KRaft broker (see `docker-compose.yml`), fine for local development but not something to run unmanaged in AWS.

**Options**: self-managed Kafka on EC2 (full control, full operational burden — broker sizing, KRaft/ZooKeeper management, patching) vs. **provisioned MSK** (managed brokers, but you still choose broker count/instance type and manage scaling) vs. **MSK Serverless** (no broker decisions at all; storage and throughput scale automatically).

**Decision**: MSK Serverless (`modules/msk`), with IAM-based client authentication (`client_authentication.sasl.iam`) — a genuine security improvement over local's unauthenticated PLAINTEXT listener, not just a lift-and-shift.

**Honestly documented gap**: MSK Serverless has no Terraform-manageable topic resource, and `auto.create.topics` is disabled by design — so `payments.created`/`payments.processed`/`payments.dlq` (6 partitions each, matching `kafka-init` locally) must be created once, out-of-band, using the Kafka CLI against this cluster's bootstrap brokers. This is the same kind of one-time operational step `kafka-init` already performs locally; it just isn't expressible as a Terraform resource for the serverless variant.

**Trade-offs**: MSK Serverless's per-request pricing model can cost more than a well-utilized provisioned cluster at high, steady throughput — accepted here because this project has no such throughput today, and "no broker sizing to get wrong" is worth more at this stage than the theoretical cost efficiency of self-managed sizing.

### Observability → Amazon Managed Prometheus + Amazon Managed Grafana

**Context**: locally, Prometheus scrapes each Go service's `/metrics` directly (pull model — see `observability/prometheus/prometheus.yml`) and Grafana reads from Prometheus, both self-hosted containers (see [ADR-003](../docs/decisions/003-observability.md)).

**Options**: self-hosted Prometheus + Grafana on ECS Fargate (identical mental model to local) vs. **Amazon Managed Prometheus (AMP) + Amazon Managed Grafana (AMG)**.

**Decision**: AMP + AMG (`modules/observability`). Self-hosting Prometheus on Fargate has a real, specific problem: Prometheus's time-series database wants persistent, low-latency local disk, and Fargate has no good persistent-disk story for that (EFS exists but adds real latency for this workload) — this isn't a stylistic preference, it's avoiding a genuine storage-architecture problem. AMP and AMG remove that entirely.

**How metrics actually reach AMP**: AMP has no way to reach into a private ECS task and scrape it the way local Prometheus does. Each Go service's `ecs-service` module instantiation optionally runs an ADOT (AWS Distro for OpenTelemetry) collector as a sidecar container (`enable_metrics_sidecar`), configured (via an SSM parameter holding its YAML config — see `environments/*/otel-collector-config.yaml.tftpl`) to scrape that service's own `/metrics` and remote-write into AMP, authenticated via the task's own IAM role (SigV4) — no static credentials.

**Honestly documented gap**: importing the *existing* `../observability/grafana/provisioning/dashboards/platform-overview.json` dashboard into AMG is **not** done by this module. The Terraform `grafana` provider needs an AMG API key/service-account token that only exists *after* the `aws_grafana_workspace` resource is created — wiring that into the same `terraform apply` would require the `grafana` provider's configuration to depend on a resource created in that same run, which Terraform does not support. The correct, honest pattern (documented in `modules/observability/main.tf`) is a deliberate second step: once the workspace exists, create an API key/token, then run a small second Terraform config (or a CI script) using the `grafana` provider to import the *same* JSON file — never re-typed into HCL.

**Trade-offs**: this is genuinely less flexible than self-hosted Grafana for advanced customization, and ties the dashboard-import step to a two-phase process instead of one `apply` — accepted because it removes Prometheus's storage problem entirely and because AMG's dashboard-import limitation is a one-time setup cost, not an ongoing one.

### Monorepo-of-modules layout: shared modules + thin environments

All real logic lives in `modules/*`; `environments/{dev,staging,prod}` are thin root modules that call those shared modules with different variable values (instance sizes, desired counts, CIDR blocks) and a different remote-state key. This mirrors the project's own existing philosophy in [ADR-004](../docs/decisions/004-monorepo.md): shared *infrastructure* modules across environments are not the same risk as shared *business logic* across independently deployable services — there's no runtime coupling here, only a DRY authoring convenience. Adding a fourth environment later is copying one `environments/` folder and adjusting its `.tfvars`, not rewriting any module.

## Directory structure

```
infrastructure/
├── README.md              — this file
├── .gitignore             — .terraform/, *.tfstate*, real *.tfvars (keeps *.tfvars.example)
├── .gitlab-ci.yml         — validate → plan → apply pipeline (see below)
├── bootstrap/             — one-time, LOCAL-state config: creates the S3 state bucket + DynamoDB lock table
├── modules/
│   ├── network/            — VPC, public/private subnets, NAT
│   ├── security/           — security groups (least-privilege, cross-referenced by SG id)
│   ├── ecs-cluster/        — Fargate cluster + shared CloudWatch log group
│   ├── ecs-service/        — REUSABLE: ECR repo + IAM + task definition + service, instantiated 3x
│   ├── alb/                — public ALB, path-based routing to the two ALB-fronted services
│   ├── rds-mariadb/        — RDS instance + Secrets Manager password
│   ├── clickhouse-ec2/     — EC2 instance running the same local Docker image + EBS + Secrets Manager password
│   ├── msk/                — MSK Serverless cluster, IAM auth
│   ├── frontend-static-site/ — S3 (private, OAC) + CloudFront
│   └── observability/      — AMP + AMG workspaces
└── environments/
    ├── dev/
    ├── staging/
    └── prod/
```

## Prerequisites

- [Terraform](https://developer.hashicorp.com/terraform) >= 1.9
- An AWS account with credentials configured (`aws configure`, or environment variables, or — recommended over long-lived keys — an assumed IAM role)
- (Not available in this environment, and not required to read/review this code)

## One-time setup: bootstrapping remote state

Terraform's own state for `dev`/`staging`/`prod` is stored remotely in S3
with DynamoDB locking (`environments/*/backend.tf`) — but that bucket
and table have to exist *before* any environment's `terraform init` can
use them. `bootstrap/` creates them, using plain local state (the
standard solution to this chicken-and-egg problem):

```bash
cd infrastructure/bootstrap
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars: state_bucket_name must be globally unique across ALL of AWS
terraform init
terraform apply
```

Then update every `environments/*/backend.tf`'s `bucket`/`dynamodb_table`
to match whatever you actually named them (they default to the same
dummy placeholder as `bootstrap/variables.tf`, so if you changed the
bucket name there, change it in all three `backend.tf` files too).

## Using an environment

```bash
cd infrastructure/environments/dev   # or staging, or prod
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars: every value in it is a dummy placeholder
terraform init
terraform plan
terraform apply
```

Each environment's Go service images are expected to already exist in
that environment's ECR repositories (see the `ecr_repository_urls`
output) before the ECS services can start — pushing the first image is
a manual `docker build && docker push` (or a CI job) against the
repository URL Terraform creates, not something `terraform apply` does
itself.

## GitLab CI

`infrastructure/.gitlab-ci.yml` defines a `validate` → `plan` → `apply`
pipeline (fmt/validate need no AWS credentials at all; `plan`/`apply`
need `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` — or, better, OIDC
federation to an IAM role — configured as GitLab CI/CD variables,
outside this repository). Because this task's scope is confined to
`infrastructure/`, this file is **not** at the repository root — wire it
up in GitLab under **Settings → CI/CD → General pipelines → "CI/CD
configuration file"**, set to `infrastructure/.gitlab-ci.yml`.

## What's deliberately not here

- No real AWS resources exist — this is code only, per the explicit
  scope of this task.
- No `terraform fmt`/`validate`/`plan` has been run against this code
  (also per explicit instruction) — review the HCL directly rather than
  trusting a "verified" claim that doesn't exist here.
- ClickHouse schema migrations and MSK topic creation are documented,
  one-time manual/CI steps, not Terraform resources (see the relevant
  decisions above for why).
- Grafana dashboard import into AMG is a documented second step, not
  part of `terraform apply` (see the observability decision above).
