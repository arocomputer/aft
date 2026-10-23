#!/bin/sh
# Shared local/CI task entry point. Only fmt edits source; no task publishes.
set -eu
cd "$(dirname "$0")"

usage() {
  echo 'usage: ./x [fmt [--check]|check|test|docs|helpers|build|release-check [vX.Y.Z]|dist vX.Y.Z]' >&2
  exit 2
}

version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
ldflags="-s -w -X main.version=$version -X github.com/fschrhunt/aft/internal/helper.Version=$version"

task=${1:-check}
if [ "$#" -gt 0 ]; then shift; fi
case "$task" in
  fmt)
    [ "$#" -le 1 ] || usage
    if [ "$#" -eq 0 ]; then
      gofmt -w cmd internal
    elif [ "$1" = --check ]; then
      files=$(gofmt -l cmd internal)
      if [ -n "$files" ]; then printf 'Run ./x fmt:\n%s\n' "$files" >&2; exit 1; fi
    else usage; fi
    ;;
  check)
    [ "$#" -eq 0 ] || usage
    ./x fmt --check
    go vet ./...
    ./x test
    ./x docs
    ./x build
    ;;
  test)
    [ "$#" -eq 0 ] || usage
    go test -race ./...
    python3 -m unittest discover -s scripts -p 'test_*.py'
    ;;
  docs)
    [ "$#" -eq 0 ] || usage
    exec python3 scripts/check_docs.py
    ;;
  helpers)
    # Cross-compile the server helper for every supported platform so the aft
    # binary can embed and install them.
    [ "$#" -eq 0 ] || usage
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
      os=${target%/*}; arch=${target#*/}
      CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$ldflags" \
        -o "internal/remote/bin/aft-helper-$os-$arch" ./cmd/aft-helper
    done
    ;;
  build)
    [ "$#" -eq 0 ] || usage
    ./x helpers
    mkdir -p bin
    go build -trimpath -ldflags "$ldflags" -o bin/aft ./cmd/aft
    go build -trimpath -ldflags "$ldflags" -o bin/aft-helper ./cmd/aft-helper
    ;;
  release-check)
    [ "$#" -le 1 ] || usage
    ./x helpers
    exec python3 scripts/release.py check "$@"
    ;;
  dist)
    [ "$#" -eq 1 ] || usage
    ./x helpers
    exec python3 scripts/release.py dist "$1"
    ;;
  *) usage ;;
esac
