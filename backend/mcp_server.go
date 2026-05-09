package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPTailEventsInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of events to return"`
}

type MCPConfigSnapshotOutput struct {
	Runtime                RuntimeSettings        `json:"runtime"`
	MCPEndpoint            string                 `json:"mcpEndpoint"`
	AuthHeaderName         string                 `json:"authHeaderName"`
	Tags                   []string               `json:"tags"`
	TrackedCommands        map[string]string      `json:"trackedCommands"`
	TrackedPaths           map[string]string      `json:"trackedPaths"`
	WrapperRules           map[string]WrapperRule `json:"wrapperRules"`
	PersistedEventLogPath  string                 `json:"persistedEventLogPath"`
	PersistedEventLogAlive bool                   `json:"persistedEventLogAlive"`
}

type MCPEventRecord struct {
	ReceivedAt time.Time      `json:"receivedAt"`
	Event      *pb.Event      `json:"event"`
	Envelope   map[string]any `json:"envelope,omitempty"`
}

type MCPTailEventsOutput struct {
	Source string           `json:"source"`
	Limit  int              `json:"limit"`
	Events []MCPEventRecord `json:"events"`
}

var (
	mcpServerOnce sync.Once
	mcpServer     *mcp.Server
)

func buildTrackedCommandsSnapshot() map[string]string {
	out := make(map[string]string)
	if trackerMaps.TrackedComms == nil {
		return out
	}
	iter := trackerMaps.TrackedComms.Iterate()
	var k [16]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		out[string(bytes.TrimRight(k[:], "\x00"))] = getTagName(tid)
	}
	return out
}

func buildTrackedPathsSnapshot() map[string]string {
	out := make(map[string]string)
	if trackerMaps.TrackedPaths == nil {
		return out
	}
	iter := trackerMaps.TrackedPaths.Iterate()
	var k [256]byte
	var tid uint32
	for iter.Next(&k, &tid) {
		out[string(bytes.TrimRight(k[:], "\x00"))] = getTagName(tid)
	}
	return out
}

func buildTagsSnapshot() []string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()

	type tagEntry struct {
		id   uint32
		name string
	}
	entries := make([]tagEntry, 0, len(tagMap))
	for id, name := range tagMap {
		entries = append(entries, tagEntry{id: id, name: name})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].id == entries[j].id {
			return entries[i].name < entries[j].name
		}
		return entries[i].id < entries[j].id
	})
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.name)
	}
	return out
}

func buildWrapperRulesSnapshot() map[string]WrapperRule {
	rulesMu.RLock()
	defer rulesMu.RUnlock()

	out := make(map[string]WrapperRule, len(wrapperRules))
	for comm, rule := range wrapperRules {
		out[comm] = rule
	}
	return out
}

func buildMCPServer() *mcp.Server {
	mcpServerOnce.Do(func() {
		server := mcp.NewServer(&mcp.Implementation{
			Name:    "agent-ebpf-filter",
			Version: "1.0.0",
		}, nil)

		mcp.AddTool(server, &mcp.Tool{
			Name:        "tail_events",
			Title:       "Tail Captured Events",
			Description: "Return the latest captured eBPF / wrapper / hook events, preferring the persistent JSONL log when it is enabled.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPTailEventsInput) (*mcp.CallToolResult, MCPTailEventsOutput, error) {
			limit := args.Limit
			if limit <= 0 {
				limit = 50
			}
			if limit > 500 {
				limit = 500
			}
			records, source, err := runtimeSettingsStore.RecentEvents(limit)
			if err != nil {
				return nil, MCPTailEventsOutput{}, err
			}
			events := make([]MCPEventRecord, 0, len(records))
			for _, record := range records {
				record = normalizeCapturedEventRecord(record)
				if record.Event == nil {
					continue
				}
				events = append(events, MCPEventRecord{
					ReceivedAt: record.ReceivedAt,
					Event:      record.Event,
					Envelope:   eventEnvelopeToJSONValue(record.Envelope),
				})
			}
			return nil, MCPTailEventsOutput{
				Source: source,
				Limit:  limit,
				Events: events,
			}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "config_snapshot",
			Title:       "Capture Configuration Snapshot",
			Description: "Return the current registry, runtime logging settings, and MCP endpoint information.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, MCPConfigSnapshotOutput, error) {
			settings := runtimeSettingsStore.Snapshot()
			logPath := settings.LogFilePath
			logAlive := false
			if settings.LogPersistenceEnabled && logPath != "" {
				if info, err := os.Stat(logPath); err == nil && !info.IsDir() {
					logAlive = true
				}
			}
			return nil, MCPConfigSnapshotOutput{
				Runtime:                settings,
				MCPEndpoint:            fmt.Sprintf("http://127.0.0.1:%d/mcp", resolveBackendPort()),
				AuthHeaderName:         "X-API-KEY",
				Tags:                   buildTagsSnapshot(),
				TrackedCommands:        buildTrackedCommandsSnapshot(),
				TrackedPaths:           buildTrackedPathsSnapshot(),
				WrapperRules:           buildWrapperRulesSnapshot(),
				PersistedEventLogPath:  logPath,
				PersistedEventLogAlive: logAlive,
			}, nil
		})

		mcpServer = server
	})

	return mcpServer
}

func buildMCPHandler() http.Handler {
	return mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
		return buildMCPServer()
	}, nil)
}
