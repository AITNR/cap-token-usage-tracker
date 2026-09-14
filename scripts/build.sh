#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
DIST_DIR="$ROOT_DIR/dist"
VERSION=${VERSION:-v1.0.0}

export PATH="/usr/local/go/bin:$PATH"
export GOPATH="${GOPATH:-/tmp/gopath}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=amd64

mkdir -p "$GOPATH"
mkdir -p "$DIST_DIR"
cd "$ROOT_DIR"
go build -buildmode=c-shared -buildvcs=false -ldflags="-s -w -X github.com/AITNR/cap-token-usage-tracker/internal/plugin.version=${VERSION}" -o "$DIST_DIR/cap-token-usage-tracker.so" .
