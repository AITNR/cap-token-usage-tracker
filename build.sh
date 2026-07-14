#!/bin/bash
export PATH=/usr/local/go/bin:/usr/bin:/bin
export GOPATH=/tmp/gopath
export GOPROXY=https://goproxy.cn,direct
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=amd64
mkdir -p /tmp/gopath
cd /mnt/d/c/cap-token-usage-tracker
go build -buildmode=c-shared -buildvcs=false -o cap-token-usage-tracker.so . 2>&1
echo "EXIT_CODE=$?"
