# ADR-004: Monorepo Layout

Status: Accepted

## Context

The platform is made up of multiple independently executable
components: three Go services (`payments-service`, `payment-processor`,
`analytics-service`), a React frontend, Docker/Compose infrastructure
definitions, observability configuration (Prometheus/Grafana
provisioning), a reserved location for future infrastructure-as-code
(`infrastructure/`, currently empty), and the architecture documentation
that ties it all together. These need to be developed, run, and reasoned
about together locally, via a single `docker compose up --build`.

## Options

### Option 1: One repository per service

Each of `payments-service`, `payment-processor`, `analytics-service`,
`frontend`, and infrastructure/observability config lives in its own
repository.

Advantages:
- Clear, enforced ownership boundary per repository.
- Each repository's CI/CD (if configured) is scoped to exactly one
  deployable unit.
- Smaller individual repositories, potentially faster to clone and
  index.

Disadvantages:
- A change that spans two services (e.g. evolving the
  `payments.created` event contract, which affects both
  `payments-service` and `payment-processor`) requires coordinated pull
  requests across multiple repositories, with no single commit that
  captures the whole change.
- Running the full local stack requires pulling and versioning multiple
  repositories in sync — there is no single `docker compose up` that
  can just work from one checkout.
- Architecture documentation either has to live in one of the
  repositories arbitrarily, or be duplicated/split across several,
  neither of which serves a reader trying to understand the whole
  system.
- Onboarding a new contributor means cloning and orienting in several
  places before they can run anything end-to-end.

### Option 2: Monorepo

All of the above lives in one repository, as it does today
(`backend/`, `frontend/`, `observability/`, `infrastructure/`, `docs/`).

Advantages:
- **Atomic cross-service changes**: a change to the `payments.created`
  contract can update both the producer and consumer side, plus the
  documentation describing it, in one commit — never a window where the
  two sides disagree because one repository's change hasn't landed yet.
- **One local environment**: `docker compose up --build` from a single
  checkout brings up the entire system.
- **Easier Docker Compose orchestration**: `docker-compose.yml` at the
  repository root can reference every service's Dockerfile by relative
  path, with no version-pinning or artifact-publishing step between
  "I changed the code" and "the whole stack picks it up."
- **Centralized architecture documentation**: `docs/architecture.md`
  describes the whole system in one place, next to the code it
  describes, and can be kept accurate against all services at once
  (exactly what this documentation effort itself relies on).
- **Simpler onboarding**: one `git clone`, one `docker compose up`.
- **Easier review of the complete system**: a reviewer can see a
  cross-service change (or verify that a change to one service didn't
  require a change elsewhere) in a single diff.

Disadvantages:
- **Repository growth**: history and checkout size accumulate across
  every component, not just the one a given contributor cares about.
- **CI complexity**: a CI pipeline (none is currently configured in this
  repository) would need path-based filtering to avoid rebuilding/testing
  every service on every change, rather than getting that scoping for
  free from separate repositories.
- **Need for ownership boundaries**: without separate repositories'
  built-in access control, ownership of each service's directory has to
  be enforced by convention (or a CODEOWNERS-style mechanism) rather
  than by repository membership.
- **Potential accidental coupling**: nothing stops a contributor from
  importing one service's package into another if they're not
  deliberate about it — the monorepo makes that *possible* in a way
  separate repositories physically wouldn't.

## Decision

Use a monorepo — this is already the repository's structure, not a
proposed change.

## Trade-offs

We accept repository growth, the need for CI path-scoping if/when CI is
introduced, and the absence of repository-enforced ownership boundaries,
in exchange for atomic cross-service changes, one working local
environment, and centralized documentation that stays honest against the
whole system rather than one slice of it.

**A monorepo does not mean the services share business logic.** Each of
`backend/payments-service`, `backend/payment-processor`, and
`backend/analytics-service` is its own Go module (its own `go.mod`), with
no shared internal package imported across service boundaries — each
service deliberately redeclares its own small wire-level event types
(see the comments in each service's `internal/domain/event.go`) rather
than importing a shared one, precisely to avoid accidental coupling
through a "shared" package that would quietly turn three independently
deployable services into a distributed monolith. Each service remains
independently:

- **Buildable** — its own `go.mod`, built by its own Dockerfile.
- **Testable** — its own `go test ./...`, with no cross-module test
  dependencies.
- **Containerized** — its own multi-stage Dockerfile, producing its own
  image.
- **Deployable** — nothing in `docker-compose.yml` or the service code
  itself assumes another service is deployed at the same time or from
  the same commit (aside from the runtime dependency of consuming a
  topic another service produces, which is a Kafka contract, not a code
  dependency).

Shared code across services is intentionally minimal — effectively none
today beyond convention (each service follows the same
`cmd/{api|processor}` + `internal/{config,domain,...}` layout, and the
same environment-variable-driven, fail-fast configuration pattern) — and
any future shared Go package should be added deliberately, not as a
convenience, since it is the one thing that could actually turn this
monorepo into a distributed monolith regardless of how the repository
itself is organized.
