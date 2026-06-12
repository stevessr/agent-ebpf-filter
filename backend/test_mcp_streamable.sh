#!/bin/bash

TOKEN=$(cat ~/.config/agent-ebpf-filter/runtime.json 2>/dev/null | grep -o '"accessToken":"[^"]*"' | cut -d'"' -f4)
PORT=8080

echo "=== Testing MCP Streamable HTTP Endpoint ==="
echo "Port: $PORT"
echo "Token: ${TOKEN:0:20}..."
echo ""

# Test 1: Initialize (with auth)
echo "Test 1: Initialize method"
curl -s -X POST http://127.0.0.1:$PORT/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -H "X-API-KEY: $TOKEN" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}' | jq . 2>/dev/null || echo "Failed or no jq"
echo ""

# Test 2: List tools
echo "Test 2: tools/list method"
curl -s -X POST http://127.0.0.1:$PORT/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -H "X-API-KEY: $TOKEN" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | jq '.result.tools[] | {name, description}' 2>/dev/null | head -30
echo ""

# Test 3: Call config_snapshot tool
echo "Test 3: Call config_snapshot tool"
curl -s -X POST http://127.0.0.1:$PORT/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -H "X-API-KEY: $TOKEN" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"config_snapshot","arguments":{}}}' | jq '.result' 2>/dev/null | head -20
echo ""

echo "=== Tests Complete ==="
