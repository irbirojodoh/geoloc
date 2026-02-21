# --- Build Stage ---
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /geoloc-api ./cmd/api

# --- Runtime Stage ---
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Non-root user for security
RUN adduser -D -u 1001 appuser

WORKDIR /app
COPY --from=builder /geoloc-api .

# Create uploads directory owned by appuser
RUN mkdir -p /app/uploads && chown appuser:appuser /app/uploads

USER appuser

EXPOSE 8080

ENTRYPOINT ["./geoloc-api"]
