#!/bin/sh
# Shared local/CI task entry point. Only fmt edits source; no task publishes.
set -eu
cd "$(dirname "$0")"

usage() {
  echo 'usage: ./x [fmt [--check]|check|test|try|bench|docs|servers|build|release-check [vX.Y.Z]|dist vX.Y.Z]' >&2
  exit 2
}

version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
ldflags="-s -w -X main.version=$version -X github.com/arocomputer/aft/internal/server.Version=$version"

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
    # The remote package test embeds the cross-compiled servers; build them if
    # they are not present yet (a fresh checkout or CI).
    [ -e internal/remote/embed/server-linux-amd64.gz ] || ./x servers
    go test -race ./...
    python3 -m unittest discover -s scripts -p 'test_*.py'
    ;;
  try)
    # The interactive test command center: build aft and open the guided
    # self-test. Everything runs on this machine in a throwaway home.
    [ "$#" -eq 0 ] || usage
    ./x build >/dev/null
    exec ./bin/aft selftest
    ;;
  bench)
    # Run the benchmarks (microbenchmarks and the end-to-end typing-latency
    # benchmark) and print the results.
    [ "$#" -eq 0 ] || usage
    exec go test -run '^$' -bench . -benchmem ./...
    ;;
  docs)
    [ "$#" -eq 0 ] || usage
    exec python3 scripts/check_docs.py
    ;;
  servers)
    # Cross-compile the server for every supported host platform and gzip each,
    # so the aft binary embeds them compressed (about half the size) and
    # decompresses on install.
    [ "$#" -eq 0 ] || usage
    for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
      os=${target%/*}; arch=${target#*/}
      out="internal/remote/embed/server-$os-$arch"
      CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$out" ./cmd/server
      gzip -9 -n -f "$out"
    done
    ;;
  build)
    [ "$#" -eq 0 ] || usage
    ./x servers
    mkdir -p bin
    go build -trimpath -ldflags "$ldflags" -o bin/aft ./cmd/aft
    ;;
  release-check)
    [ "$#" -le 1 ] || usage
    ./x servers
    exec python3 scripts/release.py check "$@"
    ;;
  dist)
    [ "$#" -eq 1 ] || usage
    ./x servers
    exec python3 scripts/release.py dist "$1"
    ;;
  *) usage ;;
esac
