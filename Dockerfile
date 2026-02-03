# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git for go mod download
RUN apk add --no-cache git

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /memcp ./cmd/memcp

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Copy Go binary
COPY --from=builder /memcp /usr/local/bin/memcp

# Expose MCP port
EXPOSE 8080

# Environment variables with defaults
ENV SURREALDB_URL=ws://localhost:8000/rpc \
    SURREALDB_NAMESPACE=knowledge \
    SURREALDB_DATABASE=graph \
    SURREALDB_USER=root \
    SURREALDB_PASS=root \
    SURREALDB_AUTH_LEVEL=root

CMD ["memcp"]
