# --- build stage -----------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/payment-processor ./cmd/processor

# --- runtime stage -----------------------------------------------------
# No Go toolchain in the final image.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /out/payment-processor /usr/local/bin/payment-processor

USER app

# Configuration is provided entirely through environment variables at
# runtime (KAFKA_BROKERS, WORKER_COUNT, WORKER_QUEUE_SIZE,
# PAYMENT_PROCESSING_TIMEOUT, etc. — see .env.example) — no credentials
# or topic names are baked into the image.
EXPOSE 9091

ENTRYPOINT ["payment-processor"]
