# Know justfile

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
export KNOW_LLM_PROVIDER := env_var_or_default("KNOW_LLM_PROVIDER", "ollama")
export KNOW_LLM_MODEL := env_var_or_default("KNOW_LLM_MODEL", "qwen2.5:1.5b")
export KNOW_EMBED_PROVIDER := env_var_or_default("KNOW_EMBED_PROVIDER", "ollama")
export KNOW_EMBED_MODEL := env_var_or_default("KNOW_EMBED_MODEL", "mxbai-embed-large")
export KNOW_EMBED_DIMENSION := env_var_or_default("KNOW_EMBED_DIMENSION", "1024")
export OLLAMA_HOST := env_var_or_default("OLLAMA_HOST", "http://localhost:11434")

# Local Whisper STT (requires: brew install whisper-cpp)
export KNOW_STT_PROVIDER := env_var_or_default("KNOW_STT_PROVIDER", "openai")
export KNOW_STT_BASE_URL := env_var_or_default("KNOW_STT_BASE_URL", "http://localhost:8090")
export KNOW_STT_MODEL := env_var_or_default("KNOW_STT_MODEL", "whisper-1")

# Chunk sizes tuned for mxbai-embed-large (512 token context ≈ 2048 chars)
export KNOW_CHUNK_THRESHOLD := env_var_or_default("KNOW_CHUNK_THRESHOLD", "1200")
export KNOW_CHUNK_TARGET_SIZE := env_var_or_default("KNOW_CHUNK_TARGET_SIZE", "1000")
export KNOW_CHUNK_MAX_SIZE := env_var_or_default("KNOW_CHUNK_MAX_SIZE", "1500")
export KNOW_EMBED_MAX_INPUT_CHARS := env_var_or_default("KNOW_EMBED_MAX_INPUT_CHARS", "2048")

# Server defaults (hardcoded to local dev — ignore shell env)
export KNOW_SERVER_PORT := "4001"
export KNOW_SERVER_URL := "http://localhost:4001"
export KNOW_DAV_DEBUG_LOG := env_var_or_default("KNOW_DAV_DEBUG_LOG", "/tmp/dav-debug.log")
export KNOW_BLOB_DIR := "./tmp-assets"

# Bootstrap / CLI defaults (hardcoded to local dev — ignore shell env)
export KNOW_BOOTSTRAP_TOKEN := "kh_0000000000000000000000000000000000000000000000000000000000000000"
export KNOW_BOOTSTRAP_VAULT_ID := "default"
export KNOW_BOOTSTRAP_VAULT_NAME := "default"
export KNOW_TOKEN := "kh_0000000000000000000000000000000000000000000000000000000000000000"

# Build directories
build_dir := "./bin"
binary := "know"

# Default recipe - show help
default:
    @just --list

# --- Build, test, run ---

# Build binary
build:
    CGO_ENABLED=1 go build -buildvcs=false -o {{build_dir}}/{{binary}} ./cmd/know

# Run all tests
test:
    CGO_ENABLED=1 go test -buildvcs=false -v ./...

# Build and run CLI command (e.g., just run serve, just run cp ./docs /)
run *args: build
    "{{build_dir}}/{{binary}}" "$@"

# Install binary to ~/go/bin and shell completions
install:
    GOBIN="$HOME/go/bin" CGO_ENABLED=1 go install -buildvcs=false ./cmd/know
    @echo "Installing shell completions..."
    "$HOME/go/bin/know" completion fish > ~/.config/fish/completions/know.fish
    @echo "Installed fish completions to ~/.config/fish/completions/know.fish"

# Install git pre-commit hook (auto-fixes: goimports, go fix, go mod tidy)
install-hooks:
    @hooks_dir=$(git rev-parse --git-common-dir)/hooks && \
    cp scripts/pre-commit "$hooks_dir/pre-commit" && \
    chmod +x "$hooks_dir/pre-commit" && \
    echo "Pre-commit hook installed to $hooks_dir/pre-commit"

# Build snapshot release locally (requires goreleaser)
snapshot:
    goreleaser release --snapshot --clean

# Build and start dev server (no auth for local dev)
dev *args: build
    "{{build_dir}}/{{binary}}" serve --no-auth "$@"

# Launch TUI agent against local dev server
know *args: build
    "{{build_dir}}/{{binary}}" agent --api-url {{KNOW_SERVER_URL}} "$@"

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
    ollama pull {{KNOW_EMBED_MODEL}}
    @echo "Embedding model ready: {{KNOW_EMBED_MODEL}}"

# Pull a small Ollama LLM for local agent chat testing
ollama-pull-llm model="qwen2.5:1.5b":
    ollama pull {{model}}
    @echo ""
    @echo "Model ready: {{model}}"
    @echo "To use it, create .env with:"
    @echo "  KNOW_LLM_PROVIDER=ollama"
    @echo "  KNOW_LLM_MODEL={{model}}"

# --- Whisper (local STT) ---

# Whisper model directory
whisper_model_dir := "./models"
whisper_model := env_var_or_default("WHISPER_MODEL", "ggml-small.en.bin")
whisper_port := "8090"

# Download Whisper model from Hugging Face
whisper-pull model=whisper_model:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p "{{whisper_model_dir}}"
    if [ -f "{{whisper_model_dir}}/{{model}}" ]; then
        echo "Model already exists: {{whisper_model_dir}}/{{model}}"
        exit 0
    fi
    curl -L --progress-bar -o "{{whisper_model_dir}}/{{model}}" \
        "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/{{model}}"
    echo "Downloaded {{model}} to {{whisper_model_dir}}/"

# Start local whisper.cpp server (requires: brew install whisper-cpp)
# --inference-path overrides the default /inference to match the OpenAI API path
whisper-server model=whisper_model: (whisper-pull model)
    whisper-server \
        --model "{{whisper_model_dir}}/{{model}}" \
        --port {{whisper_port}} \
        --host 0.0.0.0 \
        --inference-path /audio/transcriptions

# --- iOS ---

ios_project := "ios/Know.xcodeproj"
ios_scheme := "Know"
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
    xcrun simctl install {{ios_simulator_id}} {{ios_build_dir}}/Build/Products/Debug-iphonesimulator/Know.app
    xcrun simctl launch --console-pty {{ios_simulator_id}} com.know.ios

# Build and run macOS app
mac: ios-generate
    xcodebuild -project {{ios_project}} -scheme {{ios_scheme}} \
        -destination 'platform=macOS' \
        -derivedDataPath {{ios_build_dir}} build \
        CODE_SIGN_IDENTITY="" CODE_SIGNING_REQUIRED=NO CODE_SIGNING_ALLOWED=NO \
        CODE_SIGN_ENTITLEMENTS=""
    open {{ios_build_dir}}/Build/Products/Debug/Know.app
