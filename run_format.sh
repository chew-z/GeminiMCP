#!/bin/sh

# Ensure Go tools are in PATH
export PATH="/usr/local/go/bin:$PATH"

# Run gofmt to format all Go files recursively
gofmt -w .
