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
export KNOWHOW_EMBED_MODEL := env_var_or_default("KNOWHOW_EMBED_MODEL", "all-minilm:l6-v2")
export KNOWHOW_EMBED_DIMENSION := env_var_or_default("KNOWHOW_EMBED_DIMENSION", "384")

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

# Build both CLI and server
build-all: build build-server

# Run server with optional args (e.g., just server --wipe)
server *ARGS: build-server
    {{build_dir}}/{{server}} {{ARGS}}

# Install both binaries to GOPATH/bin
install:
    go install -buildvcs=false ./cmd/knowhow
    go install -buildvcs=false ./cmd/knowhow-server

# Run all tests
test:
    go test -v ./...

# Start SurrealDB, Ollama, and run the server with live-reload
dev: db-up ollama-pull
    @echo "Starting knowhow-server with Air (live-reload)..."
    @echo "CLI should use: KNOWHOW_SERVER_URL=$KNOWHOW_SERVER_URL"
    @echo "Rebuild delay: 20s after last file change"
    air

# Start dev environment and wipe database on first start
dev-reset: db-up ollama-pull
    @echo "Starting knowhow-server with Air (live-reload) - WIPING DATABASE..."
    @echo "CLI should use: KNOWHOW_SERVER_URL=$KNOWHOW_SERVER_URL"
    KNOWHOW_WIPE_DB=true air

# Run CLI command (ensures correct server URL)
[positional-arguments]
run *args: build
    {{build_dir}}/{{binary}} "$@"

# Start development environment without running the server
dev-setup: db-up ollama-pull
    @echo "SurrealDB running at localhost:8000"
    @echo "Ollama embedding model ready"
    @echo "Run 'just dev' to start the server, or '{{build_dir}}/knowhow <command>' for CLI"

# Regenerate GraphQL code
generate:
    go run github.com/99designs/gqlgen generate

# Start SurrealDB
db-up:
    docker-compose up -d surrealdb

# Stop SurrealDB
db-down:
    docker-compose down

# Pull Ollama embedding model
ollama-pull:
    @echo "Pulling embedding model $KNOWHOW_EMBED_MODEL..."
    ollama pull $KNOWHOW_EMBED_MODEL

# Remove binaries and stop containers
clean:
    rm -rf {{build_dir}}
    rm -rf tmp
    docker-compose down -v

