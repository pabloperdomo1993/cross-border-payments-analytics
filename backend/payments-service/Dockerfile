# --- build stage -----------------------------------------------------
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/payments-service ./cmd/api

# --- runtime stage -----------------------------------------------------
# No Go toolchain in the final image: alpine only provides the CA
# certificates needed for outbound TLS and a shell for debugging.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && adduser -S app -G app

COPY --from=builder /out/payments-service /usr/local/bin/payments-service

USER app

# Configuration is provided entirely through environment variables at
# runtime (HTTP_PORT, DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD) —
# no credentials are baked into the image.
EXPOSE 8080

ENTRYPOINT ["payments-service"]
