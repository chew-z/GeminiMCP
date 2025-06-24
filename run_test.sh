#!/bin/sh

export PATH="/usr/local/go/bin:$PATH"

# Run tests
go test -v ./...
