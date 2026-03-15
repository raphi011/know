#!/usr/bin/env bash
#
# go-validate.sh — Run all Go quality checks, tests, and static analysis.
# Produces a structured report for machine parsing.
#
# Exit code: 1 if any FAIL, 0 otherwise (WARNs/SKIPs don't fail).
# Env: GO_VALIDATE_AUTOFIX=false to disable auto-fixing.

set -o pipefail

AUTOFIX="${GO_VALIDATE_AUTOFIX:-true}"

# Counters
PASS=0
FAIL=0
WARN=0
SKIP=0
DID_AUTOFIX=false

# Colors (disabled if not a terminal)
if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[0;33m'
  CYAN='\033[0;36m'
  NC='\033[0m'
else
  RED='' GREEN='' YELLOW='' CYAN='' NC=''
fi

# ── Helpers ──────────────────────────────────────────────────────────────

report_pass() {
  local name="$1"; shift
  local detail="$*"
  PASS=$((PASS + 1))
  if [ -n "$detail" ]; then
    echo -e "=== ${name}: ${GREEN}PASS${NC} (${detail}) ==="
  else
    echo -e "=== ${name}: ${GREEN}PASS${NC} ==="
  fi
}

report_fail() {
  local name="$1"
  FAIL=$((FAIL + 1))
  echo -e "=== ${name}: ${RED}FAIL${NC} ==="
}

report_warn() {
  local name="$1"; shift
  local detail="$*"
  WARN=$((WARN + 1))
  if [ -n "$detail" ]; then
    echo -e "=== ${name}: ${YELLOW}WARN${NC} (${detail}) ==="
  else
    echo -e "=== ${name}: ${YELLOW}WARN${NC} ==="
  fi
}

report_skip() {
  local name="$1"; shift
  local reason="$*"
  SKIP=$((SKIP + 1))
  echo -e "=== ${name}: ${CYAN}SKIP${NC} (${reason}) ==="
}

capture_output() {
  local output
  output=$("$@" 2>&1)
  local rc=$?
  echo "$output"
  return $rc
}

indent() {
  sed 's/^/    /'
}

# Check if a tool actually works (not just a broken shim)
tool_works() {
  "$1" --help &>/dev/null || "$1" -h &>/dev/null
}

# ── Preflight ────────────────────────────────────────────────────────────

if [ ! -f "go.mod" ]; then
  echo "ERROR: go.mod not found in $(pwd)" >&2
  echo "Run this script from a Go module root." >&2
  exit 2
fi

echo "=== TOOLS ==="

available_tools=()
missing_tools=()

for tool in go gofmt staticcheck deadcode dupl gocyclo; do
  if command -v "$tool" &>/dev/null && tool_works "$tool"; then
    available_tools+=("$tool")
  else
    missing_tools+=("$tool")
  fi
done

echo "available: ${available_tools[*]}"
if [ ${#missing_tools[@]} -gt 0 ]; then
  echo "missing: ${missing_tools[*]}"
else
  echo "missing: (none)"
fi
echo ""

# ── Build Tag Detection ──────────────────────────────────────────────────

echo "=== BUILD TAGS ==="

# Standard tags to exclude (GOOS, GOARCH, common constraints)
STANDARD_TAGS="linux|darwin|windows|freebsd|openbsd|netbsd|dragonfly|solaris|plan9|aix|illumos|ios|android|js|wasm|wasip1"
STANDARD_TAGS+="|amd64|arm64|arm|386|mips|mipsle|mips64|mips64le|ppc64|ppc64le|riscv64|s390x|loong64"
STANDARD_TAGS+="|cgo|ignore|race|goexperiment\\..*"

detected_tags=()
if go_files=$(find . -name '*.go' -not -path './vendor/*' 2>/dev/null); then
  raw_tags=$(echo "$go_files" | xargs grep -h '//go:build\|// +build' 2>/dev/null | \
    sed 's|//go:build ||; s|// +build ||' | \
    tr ',' '\n' | tr ' ' '\n' | tr '|' '\n' | tr '&' '\n' | \
    sed 's/^!//; s/[()]//g' | \
    grep -vE "^(${STANDARD_TAGS})$" | \
    grep -vE '^$' | \
    sort -u)
  while IFS= read -r tag; do
    [ -n "$tag" ] && detected_tags+=("$tag")
  done <<< "$raw_tags"
fi

if [ ${#detected_tags[@]} -gt 0 ]; then
  echo "detected: ${detected_tags[*]}"
else
  echo "detected: (none)"
fi
echo ""

# ── Quality Checks ───────────────────────────────────────────────────────

# mod tidy
output=$(capture_output go mod tidy -diff 2>&1) || true
if [ -n "$output" ]; then
  if [ "$AUTOFIX" = "true" ]; then
    go mod tidy 2>/dev/null
    DID_AUTOFIX=true
    report_pass "mod_tidy" "auto-fixed"
  else
    report_fail "mod_tidy"
    echo "$output" | indent
  fi
else
  report_pass "mod_tidy"
fi
echo ""

# gofmt
output=$(gofmt -l . 2>&1) || true
if [ -n "$output" ]; then
  file_count=$(echo "$output" | wc -l | tr -d ' ')
  if [ "$AUTOFIX" = "true" ]; then
    gofmt -w . 2>/dev/null
    DID_AUTOFIX=true
    report_pass "gofmt" "auto-fixed ${file_count} files"
  else
    report_fail "gofmt"
    echo "$output" | indent
  fi
else
  report_pass "gofmt"
fi
echo ""

# go fix (Go 1.26+ modernizers)
goversion=$(go version | grep -oE 'go1\.([0-9]+)' | grep -oE '[0-9]+$')
if [ "${goversion:-0}" -ge 26 ]; then
  output=$(go fix -diff ./... 2>&1) || true
  if [ -n "$output" ]; then
    if [ "$AUTOFIX" = "true" ]; then
      go fix ./... 2>/dev/null
      DID_AUTOFIX=true
      report_pass "go_fix" "auto-fixed"
    else
      report_fail "go_fix"
      echo "$output" | indent
    fi
  else
    report_pass "go_fix"
  fi
else
  report_skip "go_fix" "requires Go 1.26+"
fi
echo ""

# go vet
output=$(capture_output go vet ./...)
rc=$?
if [ $rc -eq 0 ]; then
  report_pass "go_vet"
else
  report_fail "go_vet"
  echo "$output" | indent
fi
echo ""

# go build (retry with -buildvcs=false if VCS stamping fails)
output=$(capture_output go build ./...)
rc=$?
if [ $rc -ne 0 ] && echo "$output" | grep -q "error obtaining VCS status"; then
  output=$(capture_output go build -buildvcs=false ./...)
  rc=$?
  if [ $rc -eq 0 ]; then
    report_pass "go_build" "buildvcs=false"
  else
    report_fail "go_build"
    echo "$output" | indent
  fi
elif [ $rc -eq 0 ]; then
  report_pass "go_build"
else
  report_fail "go_build"
  echo "$output" | indent
fi
echo ""

# ── Tests ────────────────────────────────────────────────────────────────

# Unit tests
output=$(capture_output go test -count=1 ./...)
rc=$?
if [ $rc -eq 0 ]; then
  report_pass "test_unit"
  echo "$output" | grep -E '^\s*(ok|---|\?)' | indent
else
  report_fail "test_unit"
  echo "$output" | indent
fi
echo ""

# Tagged tests
for tag in "${detected_tags[@]}"; do
  output=$(capture_output go test -count=1 -tags="$tag" ./...)
  rc=$?
  if [ $rc -eq 0 ]; then
    report_pass "test_tag_${tag}"
    echo "$output" | grep -E '^\s*(ok|---|\?)' | indent
  else
    report_fail "test_tag_${tag}"
    echo "$output" | indent
  fi
  echo ""
done

# ── Static Analysis ─────────────────────────────────────────────────────

# staticcheck
if command -v staticcheck &>/dev/null; then
  output=$(capture_output staticcheck -checks=all ./...)
  rc=$?
  if [ $rc -eq 0 ] && [ -z "$output" ]; then
    report_pass "staticcheck"
  elif [ -n "$output" ]; then
    report_warn "staticcheck"
    echo "$output" | indent
  else
    report_pass "staticcheck"
  fi
else
  report_skip "staticcheck" "not installed"
  echo "    install: go install honnef.co/go/tools/cmd/staticcheck@latest"
fi
echo ""

# deadcode
if command -v deadcode &>/dev/null; then
  # deadcode requires main packages — check if any exist
  has_main=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null)
  if [ -n "$has_main" ]; then
    output=$(capture_output deadcode -test ./...)
    rc=$?
    if [ $rc -eq 0 ] && [ -z "$output" ]; then
      report_pass "deadcode"
    elif [ -n "$output" ]; then
      report_warn "deadcode"
      echo "$output" | indent
    else
      report_pass "deadcode"
    fi
  else
    report_skip "deadcode" "no main packages found"
  fi
else
  report_skip "deadcode" "not installed"
  echo "    install: go install golang.org/x/tools/cmd/deadcode@latest"
fi
echo ""

# dupl
if command -v dupl &>/dev/null && tool_works dupl; then
  output=$(capture_output dupl -threshold 50 .)
  rc=$?
  if [ -z "$output" ]; then
    report_pass "dupl"
  else
    report_warn "dupl"
    echo "$output" | indent
  fi
else
  report_skip "dupl" "not installed"
  echo "    install: go install github.com/mibk/dupl@latest"
fi
echo ""

# gocyclo
if command -v gocyclo &>/dev/null && tool_works gocyclo; then
  output=$(capture_output gocyclo -over 15 .)
  if [ -z "$output" ]; then
    report_pass "gocyclo"
  else
    count=$(echo "$output" | wc -l | tr -d ' ')
    report_warn "gocyclo" "${count} functions over 15"
    echo "$output" | indent
  fi
else
  report_skip "gocyclo" "not installed"
  echo "    install: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest"
fi
echo ""

# ── Summary ──────────────────────────────────────────────────────────────

echo "=== SUMMARY ==="
echo "PASS=${PASS} FAIL=${FAIL} WARN=${WARN} SKIP=${SKIP}"
echo "autofix=${DID_AUTOFIX}"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
