#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
go build -ldflags "-s -w -X main.version=${version}" -o yop-cli .
echo "built ./yop-cli (${version})"
