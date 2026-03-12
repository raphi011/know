# Build Go binary
FROM golang:1.26-alpine AS go-builder

WORKDIR /app

RUN apk add --no-cache git

# Download Go modules first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build single binary
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /knowhow ./cmd/knowhow

# Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 1000 knowhow \
    && adduser -S -u 1000 -G knowhow knowhow

COPY --from=go-builder /knowhow /usr/local/bin/knowhow

USER knowhow

EXPOSE 8484

ENV KNOWHOW_SERVER_PORT=8484

ENTRYPOINT ["knowhow", "serve"]
