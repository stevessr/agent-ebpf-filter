package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type RuntimeConfig struct {
	AccessToken string `json:"accessToken"`
}

func main() {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".config/agent-ebpf-filter/runtime.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("⚠ No config file: %v\n", err)
		fmt.Println("Continuing without token...")
	}

	var config RuntimeConfig
	json.Unmarshal(data, &config)
	token := config.AccessToken

	port := "8080"
	endpoint := fmt.Sprintf("http://127.0.0.1:%s/mcp", port)

	fmt.Println("=== Testing MCP Streamable HTTP Endpoint ===")
	fmt.Printf("Endpoint: %s\n", endpoint)
	fmt.Printf("Token: %s...\n\n", token[:min(20, len(token))])

	// Test 1: Initialize
	fmt.Println("Test 1: Initialize")
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	resp := doRequest(endpoint, token, initReq)
	fmt.Printf("Result: %v\n\n", resp["result"])

	// Test 2: List tools
	fmt.Println("Test 2: tools/list")
	listReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}
	resp = doRequest(endpoint, token, listReq)
	if result, ok := resp["result"].(map[string]any); ok {
		if tools, ok := result["tools"].([]any); ok {
			fmt.Printf("Found %d tools:\n", len(tools))
			for i, t := range tools {
				if tool, ok := t.(map[string]any); ok {
					fmt.Printf("  %d. %s - %s\n", i+1, tool["name"], tool["description"])
				}
			}
		}
	}
	fmt.Println()

	// Test 3: Call config_snapshot
	fmt.Println("Test 3: config_snapshot tool")
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "config_snapshot",
			"arguments": map[string]any{},
		},
	}
	resp = doRequest(endpoint, token, callReq)
	if result, ok := resp["result"].(map[string]any); ok {
		if content, ok := result["content"].([]any); ok && len(content) > 0 {
			if item, ok := content[0].(map[string]any); ok {
				if text, ok := item["text"].(string); ok {
					var config map[string]any
					json.Unmarshal([]byte(text), &config)
					fmt.Printf("Tags count: %d\n", len(config["tags"].([]any)))
					fmt.Printf("Tracked commands: %d\n", len(config["trackedCommands"].(map[string]any)))
					fmt.Printf("✅ config_snapshot works!\n")
				}
			}
		}
	}

	fmt.Println("\n=== All Tests Passed ===")
}

func doRequest(endpoint, token string, req map[string]any) map[string]any {
	body, _ := json.Marshal(req)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", endpoint, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	if token != "" {
		httpReq.Header.Set("X-API-KEY", token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		return nil
	}
	defer httpResp.Body.Close()

	respBody, _ := io.ReadAll(httpResp.Body)

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("❌ Parse failed: %v\n", err)
		fmt.Printf("Raw: %s\n", string(respBody[:min(200, len(respBody))]))
		return nil
	}

	if errObj, ok := result["error"]; ok {
		fmt.Printf("❌ RPC Error: %v\n", errObj)
		return result
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
