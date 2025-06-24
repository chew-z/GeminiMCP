#!/bin/sh

export PATH="/usr/local/go/bin:$PATH"
export HOME="/Users/rrj"
export GOLANGCI_LINT_CACHE="$HOME/Library/Caches/golangci-lint"
export GOCACHE="$HOME/.cache/go-build"

# Run linter
/Users/rrj/Projekty/Go/bin/golangci-lint run --fix ./...
