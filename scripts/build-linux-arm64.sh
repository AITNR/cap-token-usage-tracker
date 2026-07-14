#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DIST_DIR="$ROOT_DIR/dist"
VERSION=${VERSION:-v1.0.0}
RELEASE_ARTIFACT="$DIST_DIR/cap-token-usage-tracker-${VERSION}-linux-arm64.so"
INSTALL_ARTIFACT="$DIST_DIR/cap-token-usage-tracker.so"

proxy_works() {
  curl --silent --show-error --fail --max-time 5 --proxy "$1" https://proxy.golang.org/ >/dev/null 2>&1
}

resolve_proxy() {
  if [[ -n "${CLASH_PROXY_URL:-}" ]]; then
    printf '%s' "$CLASH_PROXY_URL"
    return
  fi

  local candidate="http://127.0.0.1:7897"
  if proxy_works "$candidate"; then
    printf '%s' "$candidate"
    return
  fi

  local route gateway
  route=$(ip route show default 2>/dev/null || true)
  read -r _ _ gateway _ <<<"$route"
  if [[ -n "${gateway:-}" ]]; then
    candidate="http://${gateway}:7897"
    if proxy_works "$candidate"; then
      printf '%s' "$candidate"
      return
    fi
  fi

  printf 'Clash proxy is not reachable on port 7897. Set CLASH_PROXY_URL explicitly.\n' >&2
  exit 1
}

for command in go aarch64-linux-gnu-gcc curl; do
  command -v "$command" >/dev/null || {
    printf 'Required command not found: %s\n' "$command" >&2
    exit 1
  }
done

PROXY_URL=$(resolve_proxy)
export HTTP_PROXY="$PROXY_URL"
export HTTPS_PROXY="$PROXY_URL"
export ALL_PROXY="$PROXY_URL"
export http_proxy="$PROXY_URL"
export https_proxy="$PROXY_URL"
export all_proxy="$PROXY_URL"

mkdir -p "$DIST_DIR"
cd "$ROOT_DIR"

go mod download
CGO_ENABLED=1 \
GOOS=linux \
GOARCH=arm64 \
CC=aarch64-linux-gnu-gcc \
go build \
  -buildmode=c-shared \
  -trimpath \
  -buildvcs=false \
  -ldflags="-s -w -X main.version=${VERSION}" \
  -o "$RELEASE_ARTIFACT" \
  .

cp "$RELEASE_ARTIFACT" "$INSTALL_ARTIFACT"
printf 'Built release artifact: %s\n' "$RELEASE_ARTIFACT"
printf 'Built install artifact: %s\n' "$INSTALL_ARTIFACT"
printf 'Downloads used: %s\n' "$PROXY_URL"
