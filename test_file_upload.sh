#!/bin/bash
# Test script for file upload functionality

# Build the application
echo "Building application..."
go build -o bin/mcp-gemini

# Create a test file if it doesn't exist
TEST_FILE="/tmp/test_file.go"
echo "Creating test file at $TEST_FILE..."
cat > "$TEST_FILE" << EOF
package main

import "fmt"

func main() {
    fmt.Println("Hello from test file!")
}
EOF

# Enable detailed logging
export GEMINI_LOG_LEVEL=debug

# Run the application with test environment
echo "Running application with test configuration..."
GEMINI_API_KEY=${GEMINI_API_KEY:-"your_api_key_here"} \
GEMINI_MODEL=gemini-1.5-pro-001 \
GEMINI_ENABLE_CACHING=true \
GEMINI_DEFAULT_CACHE_TTL="30m" \
./bin/mcp-gemini

# The application should now be running on the default port
# Test with a sample MCP request (requires mcp-cli or another MCP client):
#
# mcp-cli call gemini_ask --args='{"query":"Review this code", "file_paths":["/tmp/test_file.go"], "use_cache":true}' --server=http://localhost:8080
#
# Then verify in the logs that:
# 1. The file is uploaded successfully
# 2. A cache is created with the file
# 3. The file URI is properly stored and used
