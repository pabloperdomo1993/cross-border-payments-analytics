# Thin wrappers around Docker Compose and the per-module Go tooling —
# see README.md for what each target actually does end-to-end.

GO_MODULES := backend/payments-service backend/payment-processor backend/analytics-service scripts/seed

.PHONY: up down clean logs test test-race test-integration seed

up:
	docker compose up --build

down:
	docker compose down

# Removes local persistent data (MariaDB/ClickHouse/Grafana/Kafka
# volumes) in addition to stopping containers — a full local reset.
clean:
	docker compose down -v

logs:
	docker compose logs -f

# Runs each Go module's own test suite independently (they are
# separate modules, not one multi-module workspace) — no Docker
# required, matches what CI/a plain `go test ./...` per module does.
test:
	@for m in $(GO_MODULES); do \
		echo "==> go test $$m"; \
		(cd $$m && go test ./...) || exit 1; \
	done

# Same as `test`, with the race detector. Requires a C compiler
# (cgo) — see README.md if this fails with a cgo-related error.
test-race:
	@for m in $(GO_MODULES); do \
		echo "==> go test -race $$m"; \
		(cd $$m && CGO_ENABLED=1 go test -race ./...) || exit 1; \
	done

# Integration tests need the relevant real services running first,
# e.g. `docker compose up -d mariadb kafka kafka-init clickhouse`.
test-integration:
	@for m in $(GO_MODULES) tests; do \
		echo "==> go test -tags=integration $$m"; \
		(cd $$m && go test -tags=integration ./...) || exit 1; \
	done

# Generates a large deterministic synthetic dataset directly into
# MariaDB and ClickHouse (bypassing Kafka — see
# docs/architecture.md for why). Requires `make up` (or at least
# mariadb/clickhouse) to already be running.
seed:
	cd scripts/seed && go run . -target=both -count=1000000 -seed=42
