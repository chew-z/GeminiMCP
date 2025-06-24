#!/bin/sh

export PATH="/usr/local/go/bin:$PATH"

# Run gofmt to format all Go files recursively
/usr/local/go/bin/gofmt -w .
