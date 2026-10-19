#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
target=${1:-$(uname -m)}
case "$target" in
    x86_64|amd64) targets=amd64 ;;
    aarch64|arm64) targets=arm64 ;;
    all) targets='amd64 arm64' ;;
    *) echo 'Use amd64, arm64 ou all.' >&2; exit 1 ;;
esac
for arch in $targets; do
    mkdir -p "bin/linux-$arch"
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "bin/linux-$arch/finance-bot" ./cmd/bot
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "bin/linux-$arch/finance-dbtool" ./cmd/dbtool
    sha256sum "bin/linux-$arch/finance-bot" "bin/linux-$arch/finance-dbtool"
done
