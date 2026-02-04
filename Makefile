# Knowhow Makefile
BINARY_NAME := knowhow
SERVER_NAME := knowhow-server
BUILD_DIR := ./bin

# SurrealDB defaults (matching docker-compose)
export SURREALDB_URL ?= ws://localhost:8000/rpc
export SURREALDB_NAMESPACE ?= knowledge
export SURREALDB_DATABASE ?= graph
export SURREALDB_USER ?= root
export SURREALDB_PASS ?= root

# LLM defaults - using Anthropic for ask, Ollama for embeddings
export KNOWHOW_LLM_PROVIDER ?= anthropic
export KNOWHOW_LLM_MODEL ?= claude-sonnet-4-20250514
export KNOWHOW_EMBED_PROVIDER ?= ollama
export KNOWHOW_EMBED_MODEL ?= all-minilm:l6-v2
export KNOWHOW_EMBED_DIMENSION ?= 384

# Server defaults
export KNOWHOW_SERVER_PORT ?= 8080
export KNOWHOW_SERVER_URL ?= http://localhost:8080/query

.PHONY: build server build-all install test dev dev-server generate db-up db-down ollama-pull clean help

build:
	go build -buildvcs=false -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/knowhow

server:
	go build -buildvcs=false -o $(BUILD_DIR)/$(SERVER_NAME) ./cmd/knowhow-server

build-all: build server

install:
	go install -buildvcs=false ./cmd/knowhow
	go install -buildvcs=false ./cmd/knowhow-server

test:
	go test -v ./...

# Start SurrealDB, Ollama, and run the server
dev: db-up ollama-pull server
	@echo "Starting knowhow-server..."
	@$(BUILD_DIR)/$(SERVER_NAME)

# Start development environment without running the server
dev-setup: db-up ollama-pull
	@echo "SurrealDB running at localhost:8000"
	@echo "Ollama embedding model ready"
	@echo "Run 'make dev' to start the server, or '$(BUILD_DIR)/knowhow <command>' for CLI"

# Regenerate GraphQL code
generate:
	go run github.com/99designs/gqlgen generate

db-up:
	docker-compose up -d surrealdb

db-down:
	docker-compose down

ollama-pull:
	@echo "Pulling embedding model $(KNOWHOW_EMBED_MODEL)..."
	@ollama pull $(KNOWHOW_EMBED_MODEL)

clean:
	rm -rf $(BUILD_DIR)
	docker-compose down -v

help:
	@echo "Targets:"
	@echo "  build       - Build CLI binary to ./bin/knowhow"
	@echo "  server      - Build server binary to ./bin/knowhow-server"
	@echo "  build-all   - Build both CLI and server"
	@echo "  install     - Install both binaries to GOPATH/bin"
	@echo "  test        - Run all tests"
	@echo "  dev         - Start SurrealDB + Ollama + run server"
	@echo "  dev-setup   - Start SurrealDB + Ollama (without server)"
	@echo "  generate    - Regenerate GraphQL code"
	@echo "  db-up       - Start SurrealDB only"
	@echo "  db-down     - Stop SurrealDB"
	@echo "  ollama-pull - Pull embedding model (requires Ollama running)"
	@echo "  clean       - Remove binaries and stop containers"
