#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DIST_DIR="$ROOT_DIR/dist"
VERSION=${VERSION:-v1.0.0}
INSTALL_ARTIFACT=${1:-"$DIST_DIR/cap-token-usage-tracker.so"}
RELEASE_ARTIFACT=${2:-"$DIST_DIR/cap-token-usage-tracker-${VERSION}-linux-arm64.so"}
HEADER="${RELEASE_ARTIFACT%.so}.h"
CHECKSUMS="$DIST_DIR/SHA256SUMS"
SMOKE_BINARY="$DIST_DIR/abi-smoke-linux-arm64"

for command in go file readelf nm sha256sum grep cmp aarch64-linux-gnu-gcc; do
  command -v "$command" >/dev/null || {
    printf 'Required command not found: %s\n' "$command" >&2
    exit 1
  }
done

cd "$ROOT_DIR"

if [[ -n "$(gofmt -l -- *.go)" ]]; then
  printf 'Go source is not formatted.\n' >&2
  gofmt -l -- *.go >&2
  exit 1
fi

go vet ./...
CGO_ENABLED=0 go test ./...
go test ./...
if [[ "$(go env CGO_ENABLED)" == "1" ]]; then
  go test -race ./...
else
  printf 'Skipping race tests because cgo is disabled in this environment.\n' >&2
fi

for path in "$INSTALL_ARTIFACT" "$RELEASE_ARTIFACT" "$HEADER"; do
  [[ -f "$path" ]] || { printf 'Artifact not found: %s\n' "$path" >&2; exit 1; }
done
cmp -s "$INSTALL_ARTIFACT" "$RELEASE_ARTIFACT" || {
  printf 'Install and release shared libraries differ.\n' >&2
  exit 1
}

file "$INSTALL_ARTIFACT" | grep -Eq 'ELF 64-bit.*(ARM aarch64|AArch64)'
readelf -h "$INSTALL_ARTIFACT" | grep -Eq 'Class:[[:space:]]+ELF64'
readelf -h "$INSTALL_ARTIFACT" | grep -Eq 'Type:[[:space:]]+DYN'
readelf -h "$INSTALL_ARTIFACT" | grep -Eq 'Machine:[[:space:]]+AArch64'

for symbol in cliproxy_plugin_init cliproxyPluginCall cliproxyPluginFree cliproxyPluginShutdown; do
  nm -D --defined-only "$INSTALL_ARTIFACT" | grep -Eq "[[:space:]]${symbol}$" || {
    printf 'Required exported symbol missing: %s\n' "$symbol" >&2
    exit 1
  }
done

printf 'Dynamic dependencies:\n'
readelf -d "$INSTALL_ARTIFACT" | grep NEEDED || true

aarch64-linux-gnu-gcc -O2 -Wall -Wextra -o "$SMOKE_BINARY" "$ROOT_DIR/scripts/abi-smoke.c" -ldl
if command -v qemu-aarch64 >/dev/null; then
  qemu-aarch64 -L /usr/aarch64-linux-gnu "$SMOKE_BINARY" "$INSTALL_ARTIFACT"
else
  printf 'Skipping runtime ABI smoke test because qemu-aarch64 is unavailable.\n' >&2
fi

cd "$DIST_DIR"
sha256sum "$(basename "$INSTALL_ARTIFACT")" "$(basename "$RELEASE_ARTIFACT")" >"$(basename "$CHECKSUMS")"
sha256sum -c "$(basename "$CHECKSUMS")"
printf 'Verified install artifact: %s\n' "$INSTALL_ARTIFACT"
printf 'Verified release artifact: %s\n' "$RELEASE_ARTIFACT"
printf 'Checksums: %s\n' "$CHECKSUMS"
