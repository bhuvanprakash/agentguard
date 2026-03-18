# Static build with CGO for SQLite.
FROM golang:1.22-alpine AS builder

# build-base is required for CGO (sqlite)
RUN apk add --no-cache build-base

WORKDIR /app

# Copy module files first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags="-w -s" -o agentguard .

# ── Runtime stage ─────────────────────────────────────────────
FROM alpine:3.19

# ca-certificates for Https, sqlite-libs for runtime
RUN apk add --no-cache ca-certificates sqlite-libs

WORKDIR /app

COPY --from=builder /app/agentguard .
COPY --from=builder /app/policy.yaml ./policy.yaml
COPY --from=builder /app/audit/audit.db ./audit.db 2>/dev/null || :

EXPOSE 7777

CMD ["./agentguard"]
