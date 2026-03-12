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
export KNOWHOW_CHUNK_MAX_SIZE := env_var_or_default("KNOWHOW_CHUNK_MAX_SIZE", "1500")

# Server defaults
export KNOWHOW_SERVER_PORT := env_var_or_default("KNOWHOW_SERVER_PORT", "4001")
export KNOWHOW_SERVER_URL := env_var_or_default("KNOWHOW_SERVER_URL", "http://localhost:4001")
export KNOWHOW_DAV_DEBUG_LOG := env_var_or_default("KNOWHOW_DAV_DEBUG_LOG", "/tmp/dav-debug.log")

# Bootstrap / CLI defaults (stable dev token + vault)
export KNOWHOW_BOOTSTRAP_TOKEN := env_var_or_default("KNOWHOW_BOOTSTRAP_TOKEN", "kh_0000000000000000000000000000000000000000000000000000000000000000")
export KNOWHOW_BOOTSTRAP_VAULT_ID := env_var_or_default("KNOWHOW_BOOTSTRAP_VAULT_ID", "default")
export KNOWHOW_BOOTSTRAP_VAULT_NAME := env_var_or_default("KNOWHOW_BOOTSTRAP_VAULT_NAME", "default")
export KNOWHOW_TOKEN := env_var_or_default("KNOWHOW_TOKEN", "kh_0000000000000000000000000000000000000000000000000000000000000000")

# Build directories
build_dir := "./bin"
binary := "knowhow"

# Default recipe - show help
default:
    @just --list

# --- Build, test, run ---

# Build binary
build:
    go build -buildvcs=false -o {{build_dir}}/{{binary}} ./cmd/knowhow

# Run all tests
test:
    go test -buildvcs=false -v ./...

# Build and run CLI command (e.g., just run serve, just run cp ./docs /)
run *args: build
    "{{build_dir}}/{{binary}}" "$@"

# Install binary to ~/go/bin (explicit GOBIN avoids mise's GOBIN override)
install:
    GOBIN="$HOME/go/bin" go install -buildvcs=false ./cmd/knowhow

# Build snapshot release locally (requires goreleaser)
snapshot:
    goreleaser release --snapshot --clean

# Build and start dev server (no auth for local dev)
dev: build
    "{{build_dir}}/{{binary}}" serve --no-auth

# Bootstrap DB (wipe + create user/vault/token from env vars)
bootstrap: build
    {{build_dir}}/{{binary}} db wipe
    {{build_dir}}/{{binary}} db seed

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

# --- Ollama ---

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

# --- iOS ---

ios_project := "ios/Knowhow.xcodeproj"
ios_scheme := "Knowhow"
ios_simulator_id := env_var_or_default("IOS_SIMULATOR_ID", "3172C353-0D1C-42FF-BE32-1F123B8D4954")
ios_build_dir := "ios/build"

# Boot iOS simulator
ios-sim:
    xcrun simctl boot {{ios_simulator_id}} 2>/dev/null || true
    open -a Simulator

# Generate Xcode project from project.yml
ios-generate:
    cd ios && xcodegen generate

# Build iOS app for simulator
ios-build: ios-generate
    xcodebuild -project {{ios_project}} -scheme {{ios_scheme}} \
        -destination 'platform=iOS Simulator,id={{ios_simulator_id}}' \
        -derivedDataPath {{ios_build_dir}} build

# Build and run iOS app on simulator
ios-run: ios-build
    xcrun simctl boot {{ios_simulator_id}} 2>/dev/null || true
    xcrun simctl install {{ios_simulator_id}} {{ios_build_dir}}/Build/Products/Debug-iphonesimulator/Knowhow.app
    xcrun simctl launch --console-pty {{ios_simulator_id}} com.knowhow.ios
