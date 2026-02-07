# Stage 1: Build web frontend
FROM oven/bun:1-alpine AS web-builder

WORKDIR /app/web

COPY web/package.json web/bun.lock* ./
RUN bun install

COPY web/ ./
RUN bun run build

# Stage 2: Build Go binaries
FROM golang:1.25-alpine AS go-builder

WORKDIR /app

RUN apk add --no-cache git

# Download Go modules first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/embed.go ./web/embed.go

# Copy built web assets from previous stage
COPY --from=web-builder /app/web/dist/ ./web/dist/

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /knowhow-server ./cmd/knowhow-server
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /knowhow ./cmd/knowhow

# Stage 3: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S knowhow \
    && adduser -S knowhow -G knowhow

COPY --from=go-builder /knowhow-server /usr/local/bin/knowhow-server
COPY --from=go-builder /knowhow /usr/local/bin/knowhow

USER knowhow

EXPOSE 8484

ENV KNOWHOW_SERVER_PORT=8484

ENTRYPOINT ["knowhow-server"]
