# Knowhow justfile

# Load .env file if present
set dotenv-load := true
set positional-arguments := true

# SurrealDB defaults (matching docker-compose)
export SURREALDB_URL := env_var_or_default("SURREALDB_URL", "ws://localhost:4002/rpc")
export SURREALDB_NAMESPACE := env_var_or_default("SURREALDB_NAMESPACE", "knowledge")
export SURREALDB_DATABASE := env_var_or_default("SURREALDB_DATABASE", "graph")
export SURREALDB_USER := env_var_or_default("SURREALDB_USER", "root")
export SURREALDB_PASS := env_var_or_default("SURREALDB_PASS", "root")

# LLM defaults - using Ollama for local dev
export KNOWHOW_LLM_PROVIDER := env_var_or_default("KNOWHOW_LLM_PROVIDER", "ollama")
export KNOWHOW_LLM_MODEL := env_var_or_default("KNOWHOW_LLM_MODEL", "qwen2.5:1.5b")
export KNOWHOW_EMBED_PROVIDER := env_var_or_default("KNOWHOW_EMBED_PROVIDER", "ollama")
export KNOWHOW_EMBED_MODEL := env_var_or_default("KNOWHOW_EMBED_MODEL", "mxbai-embed-large")
export KNOWHOW_EMBED_DIMENSION := env_var_or_default("KNOWHOW_EMBED_DIMENSION", "1024")
export OLLAMA_HOST := env_var_or_default("OLLAMA_HOST", "http://localhost:11434")

# Chunk sizes tuned for mxbai-embed-large (512 token context ≈ 2048 chars)
export KNOWHOW_CHUNK_THRESHOLD := env_var_or_default("KNOWHOW_CHUNK_THRESHOLD", "1200")
export KNOWHOW_CHUNK_TARGET_SIZE := env_var_or_default("KNOWHOW_CHUNK_TARGET_SIZE", "1000")
export KNOWHOW_CHUNK_MIN_SIZE := env_var_or_default("KNOWHOW_CHUNK_MIN_SIZE", "200")
export KNOWHOW_CHUNK_MAX_SIZE := env_var_or_default("KNOWHOW_CHUNK_MAX_SIZE", "1500")

# Server defaults
export KNOWHOW_SERVER_PORT := env_var_or_default("KNOWHOW_SERVER_PORT", "4001")
export KNOWHOW_SERVER_URL := env_var_or_default("KNOWHOW_SERVER_URL", "http://localhost:4001/query")

# Web defaults
export SESSION_SECRET := env_var_or_default("SESSION_SECRET", "dev-secret-not-for-production-use-only")

# Bootstrap / CLI defaults (stable dev token + vault)
export KNOWHOW_BOOTSTRAP_TOKEN := env_var_or_default("KNOWHOW_BOOTSTRAP_TOKEN", "kh_0000000000000000000000000000000000000000000000000000000000000000")
export KNOWHOW_BOOTSTRAP_VAULT_ID := env_var_or_default("KNOWHOW_BOOTSTRAP_VAULT_ID", "default")
export KNOWHOW_BOOTSTRAP_VAULT_NAME := env_var_or_default("KNOWHOW_BOOTSTRAP_VAULT_NAME", "default")
export KNOWHOW_TOKEN := env_var_or_default("KNOWHOW_TOKEN", "kh_0000000000000000000000000000000000000000000000000000000000000000")

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

# Bootstrap DB (wipe + create user/vault/token from env vars)
bootstrap: build-bootstrap db-up
    {{build_dir}}/bootstrap

# Build MCP server binary
build-mcp:
    go build -buildvcs=false -o {{build_dir}}/knowhow-mcp ./cmd/knowhow-mcp

# Build all binaries
build-all: build build-server build-bootstrap build-mcp

# Run server with optional args (e.g., just server --wipe)
server *args: build-server
    "{{build_dir}}/{{server}}" "$@"

# Install both binaries to GOPATH/bin
install:
    go install -buildvcs=false ./cmd/knowhow
    go install -buildvcs=false ./cmd/knowhow-server

# Run all tests
test:
    go test -buildvcs=false -v ./...

# Start dev environment with live-reload
dev: db-up
    air

# Start dev environment and wipe database on first start
dev-reset: db-up
    KNOWHOW_WIPE_DB=true air

# Run CLI command (ensures correct server URL)
run *args: build
    "{{build_dir}}/{{binary}}" "$@"

# Start development environment without running the server
dev-setup: db-up
    @echo "SurrealDB running at localhost:4002"
    @echo "Run 'just dev' to start the server, or '{{build_dir}}/knowhow <command>' for CLI"

# Regenerate GraphQL code
generate:
    go run github.com/99designs/gqlgen generate --config gqlgen.yml

# Pull Ollama models (embedding + LLM)
ollama-pull:
    ollama pull {{KNOWHOW_EMBED_MODEL}}
    @echo "Embedding model ready: {{KNOWHOW_EMBED_MODEL}}"

# Pull a small Ollama LLM for local agent chat testing
ollama-pull-llm model="qwen2.5:1.5b":
    ollama pull {{model}}
    @echo ""
    @echo "Model ready: {{model}}"
    @echo "To use it, create .env with:"
    @echo "  KNOWHOW_LLM_PROVIDER=ollama"
    @echo "  KNOWHOW_LLM_MODEL={{model}}"

# Start SurrealDB
db-up:
    docker compose up -d surrealdb

# Stop SurrealDB
db-down:
    docker compose down

# Remove binaries and stop containers
clean:
    rm -rf {{build_dir}}
    rm -rf tmp
    docker compose down -v

# --- Web frontend ---

# Install web dependencies
web-install:
    cd web && bun install

# Start web dev server
web-dev:
    cd web && bun run dev --port 4000

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

# --- Unified dev ---

# Start all services (SurrealDB + Go server + Web dev)
dev-all: db-up
    #!/usr/bin/env bash
    set -e
    trap 'kill 0' EXIT
    air &
    cd web && bun run dev --port 4000 &
    wait

# --- Prod-local Docker stack (Bedrock via Teleport proxy) ---

# Start prod stack (persistent DB + Bedrock server + web, no auth)
prod:
    docker compose -f docker-compose.prod.yml up --build -d

# Stop prod stack
prod-down:
    docker compose -f docker-compose.prod.yml down

# Reset prod stack (wipe DB + data)
prod-reset:
    docker compose -f docker-compose.prod.yml down -v
    rm -rf data/surreal

# Bootstrap prod DB (start only surrealdb, run bootstrap, stop)
prod-bootstrap: build-bootstrap
    #!/usr/bin/env bash
    set -e
    docker compose -f docker-compose.prod.yml up -d surrealdb
    echo "Waiting for SurrealDB to be healthy..."
    until docker inspect --format='{{"{{.State.Health.Status}}"}}' knowhow-surrealdb-prod 2>/dev/null | grep -q healthy; do
        sleep 1
    done
    echo "Running bootstrap..."
    SURREALDB_URL="ws://localhost:5002/rpc" {{build_dir}}/bootstrap
    echo "Bootstrap complete. Run 'just prod' to start the full stack."

# Run all tests (Go + Web)
test-all: test web-test
