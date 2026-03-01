# Knowhow justfile

# Load .env file if present
set dotenv-load := true

# SurrealDB defaults (matching docker-compose)
export SURREALDB_URL := env_var_or_default("SURREALDB_URL", "ws://localhost:8000/rpc")
export SURREALDB_NAMESPACE := env_var_or_default("SURREALDB_NAMESPACE", "knowledge")
export SURREALDB_DATABASE := env_var_or_default("SURREALDB_DATABASE", "graph")
export SURREALDB_USER := env_var_or_default("SURREALDB_USER", "root")
export SURREALDB_PASS := env_var_or_default("SURREALDB_PASS", "root")

# LLM defaults - using Anthropic for ask, Ollama for embeddings
export KNOWHOW_LLM_PROVIDER := env_var_or_default("KNOWHOW_LLM_PROVIDER", "anthropic")
export KNOWHOW_LLM_MODEL := env_var_or_default("KNOWHOW_LLM_MODEL", "claude-sonnet-4-20250514")
export KNOWHOW_EMBED_PROVIDER := env_var_or_default("KNOWHOW_EMBED_PROVIDER", "ollama")
export KNOWHOW_EMBED_MODEL := env_var_or_default("KNOWHOW_EMBED_MODEL", "bge-m3")
export KNOWHOW_EMBED_DIMENSION := env_var_or_default("KNOWHOW_EMBED_DIMENSION", "1024")

# Server defaults
export KNOWHOW_SERVER_PORT := env_var_or_default("KNOWHOW_SERVER_PORT", "8484")
export KNOWHOW_SERVER_URL := env_var_or_default("KNOWHOW_SERVER_URL", "http://localhost:8484/query")

# Build directories
build_dir := "./bin"
binary := "knowhow"
server := "knowhow-server"

# Default recipe - show help
default:
    @just --list

# Build CLI binary
build:
    go build -buildvcs=false -o {{build_dir}}/{{binary}} ./cmd/knowhow

# Build server binary
build-server:
    go build -buildvcs=false -o {{build_dir}}/{{server}} ./cmd/knowhow-server

# Build bootstrap script
build-bootstrap:
    go build -buildvcs=false -o {{build_dir}}/bootstrap ./cmd/bootstrap

# Build all binaries
build-all: build build-server build-bootstrap

# Run server with optional args (e.g., just server --wipe)
server *ARGS: build-server
    {{build_dir}}/{{server}} {{ARGS}}

# Install both binaries to GOPATH/bin
install:
    go install -buildvcs=false ./cmd/knowhow
    go install -buildvcs=false ./cmd/knowhow-server

# Run all tests
test:
    go test -buildvcs=false -v ./...

# Start dev environment with live-reload
dev: db-up ollama-pull
    air

# Start dev environment and wipe database on first start
dev-reset: db-up ollama-pull
    KNOWHOW_WIPE_DB=true air

# Run CLI command (ensures correct server URL)
run *args: build
    {{build_dir}}/{{binary}} {{args}}

# Start development environment without running the server
dev-setup: db-up ollama-pull
    @echo "SurrealDB running at localhost:8000"
    @echo "Ollama embedding model ready"
    @echo "Run 'just dev' to start the server, or '{{build_dir}}/knowhow <command>' for CLI"

# Regenerate GraphQL code
generate:
    go run github.com/99designs/gqlgen generate --config gqlgen.yml

# Start SurrealDB
db-up:
    docker-compose up -d surrealdb

# Stop SurrealDB
db-down:
    docker-compose down

# Pull Ollama embedding model (only if using Ollama provider)
ollama-pull:
    #!/usr/bin/env bash
    if [ "$KNOWHOW_EMBED_PROVIDER" = "ollama" ]; then
        echo "Pulling embedding model $KNOWHOW_EMBED_MODEL..."
        ollama pull "$KNOWHOW_EMBED_MODEL"
    else
        echo "Skipping Ollama pull (using $KNOWHOW_EMBED_PROVIDER provider)"
    fi

# Remove binaries and stop containers
clean:
    rm -rf {{build_dir}}
    rm -rf tmp
    docker-compose down -v

# --- Web frontend ---

# Install web dependencies
web-install:
    cd web && bun install

# Start web dev server
web-dev:
    cd web && bun run dev

# Build web for production
web-build:
    cd web && bun run build

# Run web tests (unit + storybook)
web-test:
    cd web && bun run test:ci

# Run web E2E tests
web-test-e2e:
    cd web && bun run test:e2e

# Lint + typecheck web
web-lint:
    cd web && bun run lint && bun run typecheck

# Run web DB migrations
web-db-migrate:
    cd web && bun run db:migrate

# Run web DB seed
web-db-seed:
    cd web && bun run db:seed

# --- Unified dev ---

# Start all databases (SurrealDB + PostgreSQL)
db-all:
    docker-compose up -d surrealdb postgres

# Start all services (SurrealDB + PostgreSQL + Go server + Web dev)
dev-all: db-all ollama-pull
    #!/usr/bin/env bash
    set -e
    trap 'kill 0' EXIT
    air &
    cd web && bun run dev &
    wait

# Run all tests (Go + Web)
test-all: test web-test

