# --- Build Stage ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build the indexer binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /geoloc-indexer ./cmd/indexer

# --- Runtime Stage ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl

# Non-root user for security
RUN adduser -D -u 1001 appuser

WORKDIR /app
COPY --from=builder /geoloc-indexer .
COPY --from=builder /app/migrations/elasticsearch ./migrations/elasticsearch

USER appuser

ENTRYPOINT ["./geoloc-indexer"]
