package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const defaultVisualBlocksSystemPrompt = "You are an eBPF kernel-defense policy compiler. Convert natural language into a safe low-code block graph. Return strict JSON only."

var visualLLMAllowedTriggers = map[string]string{
	"process":             "process",
	"bprm":                "process",
	"bprm_check":          "process",
	"bprm_check_security": "process",
	"file_open":           "file_open",
	"open":                "file_open",
	"read":                "file_open",
	"mkdir":               "mkdir",
	"inode_mkdir":         "mkdir",
	"file_create":         "file_create",
	"create":              "file_create",
	"inode_create":        "file_create",
	"rmdir":               "rmdir",
	"inode_rmdir":         "rmdir",
	"symlink":             "symlink",
	"inode_symlink":       "symlink",
	"unlink":              "unlink",
	"delete":              "unlink",
	"inode_unlink":        "unlink",
	"socket":              "socket_connect",
	"connect":             "socket_connect",
	"socket_connect":      "socket_connect",
	"inode_mknod":         "inode_mknod",
	"mknod":               "inode_mknod",
	"file_mprotect":       "file_mprotect",
	"mprotect":            "file_mprotect",
	"rwx":                 "file_mprotect",
	"inode_rename":        "inode_rename",
	"rename":              "inode_rename",
}

var visualLLMAllowedConditionFields = map[string]string{
	"comm":     "comm",
	"command":  "comm",
	"process":  "comm",
	"pid":      "pid",
	"uid":      "uid",
	"user":     "uid",
	"gid":      "gid",
	"group":    "gid",
	"basename": "basename",
	"file":     "basename",
	"filename": "basename",
	"name":     "basename",
	"port":     "port",
	"ipv4":     "ipv4",
	"ip":       "ipv4",
	"address":  "ipv4",
}

type visualBlocksLLMCompileRequest struct {
	Prompt  string         `json:"prompt"`
	Current map[string]any `json:"current,omitempty"`
}

type visualBlocksLogicNode struct {
	ID       string                  `json:"id"`
	Type     string                  `json:"type"`
	Field    string                  `json:"field,omitempty"`
	Operator string                  `json:"operator,omitempty"`
	Value    string                  `json:"value,omitempty"`
	Children []visualBlocksLogicNode `json:"children,omitempty"`
}

type visualBlocksLLMCompileResponse struct {
	Trigger    string                `json:"trigger"`
	Action     string                `json:"action"`
	Conditions visualBlocksLogicNode `json:"conditions"`
	MapMode    string                `json:"mapMode"`
	MapKey     string                `json:"mapKey"`
	MapLimit   int                   `json:"mapLimit"`
	Reasoning  string                `json:"reasoning,omitempty"`
	Warnings   []string              `json:"warnings,omitempty"`
	Model      string                `json:"model,omitempty"`
	RawContent string                `json:"rawContent,omitempty"`
}

func handlePluginVisualLLMCompile(c *gin.Context) {
	var req visualBlocksLLMCompileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
		return
	}

	resp, err := compileVisualBlocksWithLLM(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func compileVisualBlocksWithLLM(ctx context.Context, req visualBlocksLLMCompileRequest) (*visualBlocksLLMCompileResponse, error) {
	cfg := currentMLConfig()
	if !llmScoringConfigured() {
		return nil, errors.New("LLM is not configured; enable ML LLM base URL and model in Runtime Config / ML")
	}

	endpoint, err := normalizedLLMCompletionURL(cfg.LlmBaseURL)
	if err != nil {
		return nil, err
	}

	prompt := buildVisualBlocksLLMPrompt(req)
	openAIReq := openAIChatRequest{
		Model: strings.TrimSpace(cfg.LlmModel),
		Messages: []openAIChatMessage{
			{Role: "system", Content: defaultVisualBlocksSystemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: clampFloat64(cfg.LlmTemperature, 0, 2),
		MaxTokens:   clampInt(maxInt(cfg.LlmMaxTokens, 1024), 256, 4096),
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if key := strings.TrimSpace(cfg.LlmAPIKey); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}

	client := &http.Client{Timeout: time.Duration(maxInt(cfg.LlmTimeoutSeconds, 45)) * time.Second}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		msg := strings.TrimSpace(string(payload))
		if msg == "" {
			msg = httpResp.Status
		}
		return nil, fmt.Errorf("LLM API request failed: %s: %s", httpResp.Status, msg)
	}

	var openAIResp openAIChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&openAIResp); err != nil {
		return nil, err
	}
	if openAIResp.Error != nil && strings.TrimSpace(openAIResp.Error.Message) != "" {
		return nil, errors.New(openAIResp.Error.Message)
	}
	if len(openAIResp.Choices) == 0 {
		return nil, errors.New("LLM API returned no choices")
	}

	content := strings.TrimSpace(openAIResp.Choices[0].Message.Content)
	compiled, err := parseVisualBlocksLLMContent(content)
	if err != nil {
		return nil, err
	}
	compiled.Model = openAIReq.Model
	compiled.RawContent = content
	return compiled, nil
}

func buildVisualBlocksLLMPrompt(req visualBlocksLLMCompileRequest) string {
	currentJSON := "{}"
	if len(req.Current) > 0 {
		if data, err := json.MarshalIndent(req.Current, "", "  "); err == nil {
			currentJSON = string(data)
		}
	}
	return fmt.Sprintf(`Convert the user's kernel-defense request into the visual block graph used by agent-ebpf-filter.

Return STRICT JSON only with this schema:
{
  "trigger": "process|file_open|mkdir|file_create|rmdir|symlink|unlink|socket_connect|inode_mknod|file_mprotect|inode_rename",
  "action": "BLOCK|ALERT|KILL",
  "conditions": {"id":"root", "type":"AND|OR", "children":[{"id":"cond-1", "type":"CONDITION", "field":"comm|pid|uid|gid|basename|port|ipv4", "operator":"==|!=|starts_with|ends_with", "value":"..."}]},
  "mapMode": "NONE|COUNTER|BLOCKLIST",
  "mapKey": "pid|uid|comm",
  "mapLimit": 10,
  "reasoning": "short Chinese explanation",
  "warnings": ["optional Chinese warning"]
}

Rules:
- Use trigger=socket_connect whenever matching port or ipv4.
- port/ipv4 fields are only valid for socket_connect.
- basename is for file/process name hooks, but not unlink because this UI uses kprobe/do_unlinkat for unlink.
- unlink cannot use BLOCK; use ALERT unless the user explicitly asks to kill.
- Keep total CONDITION leaves <= 8 and nesting depth <= 5.
- Values must not contain quotes, backslashes, or newlines.
- Do not generate C code. Only return the block JSON.

Current workspace context, if useful:
%s

User request:
%s
`, currentJSON, req.Prompt)
}
