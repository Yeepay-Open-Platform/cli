#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
go build -ldflags "-s -w -X github.com/Yeepay-Open-Platform/cli/internal/build.Version=${version} -X github.com/Yeepay-Open-Platform/cli/internal/build.Date=$(date +%Y-%m-%d)" -o yop-cli .
echo "built ./yop-cli (${version})"
