# --- build stage -----------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/analytics-service ./cmd/api

# --- runtime stage -----------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /out/analytics-service /usr/local/bin/analytics-service

USER app

# Configuration is provided entirely through environment variables at
# runtime (HTTP_PORT, CLICKHOUSE_ADDR, KAFKA_BROKERS, etc. — see
# .env.example) — no credentials are baked into the image.
EXPOSE 8082

ENTRYPOINT ["analytics-service"]
