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
ARG VERSION=dev
ARG COMMIT=none

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /know ./cmd/know

# Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata ffmpeg \
    && addgroup -S -g 1000 know \
    && adduser -S -u 1000 -G know know

COPY --from=go-builder /know /usr/local/bin/know

USER know

EXPOSE 8484

ENV KNOW_SERVER_PORT=8484

ENTRYPOINT ["know", "serve"]
