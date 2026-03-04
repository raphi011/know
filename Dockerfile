# Build Go binaries
FROM golang:1.26-alpine AS go-builder

WORKDIR /app

RUN apk add --no-cache git

# Download Go modules first (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /knowhow-server ./cmd/knowhow-server
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /knowhow ./cmd/knowhow
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -o /bootstrap ./cmd/bootstrap

# Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 1000 knowhow \
    && adduser -S -u 1000 -G knowhow knowhow

COPY --from=go-builder /knowhow-server /usr/local/bin/knowhow-server
COPY --from=go-builder /knowhow /usr/local/bin/knowhow
COPY --from=go-builder /bootstrap /usr/local/bin/bootstrap

USER knowhow

EXPOSE 8484

ENV KNOWHOW_SERVER_PORT=8484

ENTRYPOINT ["knowhow-server"]
