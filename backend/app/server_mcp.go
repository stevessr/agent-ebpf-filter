package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- moved from backend/zz_merged_backend.go section server_mcp.go ----

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

type MCPAddTrackedCommandInput struct {
	Command string `json:"command" jsonschema:"required,description=command name to track"`
	Tag     string `json:"tag" jsonschema:"required,description=tag name for this command"`
}

type MCPAddTrackedPathInput struct {
	Path string `json:"path" jsonschema:"required,description=absolute path to track"`
	Tag  string `json:"tag" jsonschema:"required,description=tag name for this path"`
}

type MCPQueryEventsInput struct {
	EventType string `json:"eventType,omitempty" jsonschema:"filter by event type (e.g., execve, openat, connect)"`
	Comm      string `json:"comm,omitempty" jsonschema:"filter by command name"`
	PID       uint32 `json:"pid,omitempty" jsonschema:"filter by process ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum events to return (default 100, max 500)"`
}

type MCPNetworkFlowsOutput struct {
	Flows []NetworkFlowSummary `json:"flows"`
}

type MCPSystemHealthOutput struct {
	CollectorHealth       CollectorHealthResponse   `json:"collectorHealth"`
	TracepointBootstrap   TracepointBootstrapStatus `json:"tracepointBootstrap"`
	OTelExporter          OTelHealthResponse        `json:"otelExporter"`
	CgroupSandboxAttached bool                      `json:"cgroupSandboxAttached"`
	LSMEnforcerAttached   bool                      `json:"lsmEnforcerAttached"`
}

type MCPBlockDestinationInput struct {
	IP   string `json:"ip,omitempty" jsonschema:"IPv4 or IPv6 address to block"`
	Port uint16 `json:"port,omitempty" jsonschema:"TCP/UDP port to block (1-65535)"`
}

type MCPBlockCgroupInput struct {
	PID uint32 `json:"pid" jsonschema:"required,description=process ID whose cgroup should be blocked"`
}

type MCPBlockFileInput struct {
	Path     string `json:"path,omitempty" jsonschema:"exact executable path to block"`
	Basename string `json:"basename,omitempty" jsonschema:"file or directory basename to block"`
	IsExec   bool   `json:"isExec,omitempty" jsonschema:"true if blocking executable, false for file operations"`
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
			records, source, err := runtimeSettingsStore.RecentEventsContext(ctx, limit)
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
				MCPEndpoint:            fmt.Sprintf("http://127.0.0.1:%d/mcp", platform.ResolveBackendPort()),
				AuthHeaderName:         "X-API-KEY",
				Tags:                   buildTagsSnapshot(),
				TrackedCommands:        buildTrackedCommandsSnapshot(),
				TrackedPaths:           buildTrackedPathsSnapshot(),
				WrapperRules:           buildWrapperRulesSnapshot(),
				PersistedEventLogPath:  logPath,
				PersistedEventLogAlive: logAlive,
			}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "add_tracked_command",
			Title:       "Add Tracked Command",
			Description: "Register a new command name for tracking with the specified tag.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPAddTrackedCommandInput) (*mcp.CallToolResult, map[string]any, error) {
			tid := getTagID(args.Tag)
			var k [16]byte
			copy(k[:], args.Command)
			if err := trackerMaps.TrackedComms.Put(k, tid); err != nil {
				return nil, nil, fmt.Errorf("failed to add tracked command: %w", err)
			}
			return nil, map[string]any{"success": true, "command": args.Command, "tag": args.Tag}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "add_tracked_path",
			Title:       "Add Tracked Path",
			Description: "Register a new file path for tracking with the specified tag.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPAddTrackedPathInput) (*mcp.CallToolResult, map[string]any, error) {
			tid := getTagID(args.Tag)
			var k [256]byte
			copy(k[:], args.Path)
			if err := trackerMaps.TrackedPaths.Put(k, tid); err != nil {
				return nil, nil, fmt.Errorf("failed to add tracked path: %w", err)
			}
			return nil, map[string]any{"success": true, "path": args.Path, "tag": args.Tag}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "query_events",
			Title:       "Query Filtered Events",
			Description: "Query recent events with optional filters for event type, command name, or PID.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPQueryEventsInput) (*mcp.CallToolResult, MCPTailEventsOutput, error) {
			limit := args.Limit
			if limit <= 0 {
				limit = 100
			}
			if limit > 500 {
				limit = 500
			}
			records, source, err := runtimeSettingsStore.RecentEventsContext(ctx, limit*2)
			if err != nil {
				return nil, MCPTailEventsOutput{}, err
			}
			events := make([]MCPEventRecord, 0)
			for _, record := range records {
				record = normalizeCapturedEventRecord(record)
				if record.Event == nil {
					continue
				}
				if args.EventType != "" && record.Event.Type != args.EventType {
					continue
				}
				if args.Comm != "" && record.Event.Comm != args.Comm {
					continue
				}
				if args.PID != 0 && record.Event.Pid != args.PID {
					continue
				}
				events = append(events, MCPEventRecord{
					ReceivedAt: record.ReceivedAt,
					Event:      record.Event,
					Envelope:   eventEnvelopeToJSONValue(record.Envelope),
				})
				if len(events) >= limit {
					break
				}
			}
			return nil, MCPTailEventsOutput{Source: source, Limit: limit, Events: events}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_network_flows",
			Title:       "Get Network Flows",
			Description: "Return current network flow summaries with TCP/UDP connection states.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, MCPNetworkFlowsOutput, error) {
			flows := currentNetworkFlowAggregator().Snapshot()
			return nil, MCPNetworkFlowsOutput{Flows: flows}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_system_health",
			Title:       "Get System Health",
			Description: "Return comprehensive system health including collector metrics, bootstrap status, OTLP exporter, and enforcement state.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, MCPSystemHealthOutput, error) {
			return nil, MCPSystemHealthOutput{
				CollectorHealth:       collectorMetricsStore.Snapshot(),
				TracepointBootstrap:   bootstrapTracepointStatusStore.Snapshot(),
				OTelExporter:          otelExporterStore.Snapshot(),
				CgroupSandboxAttached: currentCgroupSandboxSnapshot().attached(),
				LSMEnforcerAttached:   currentLsmEnforcerSnapshot().attached(),
			}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "block_network_destination",
			Title:       "Block Network Destination",
			Description: "Add a cgroup-level network block for an IPv4/IPv6 address or TCP/UDP port. Requires policyManagementEnabled runtime flag.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPBlockDestinationInput) (*mcp.CallToolResult, map[string]any, error) {
			settings := runtimeSettingsStore.Snapshot()
			if !settings.PolicyManagementEnabled {
				return nil, nil, fmt.Errorf("policy management is disabled in runtime settings")
			}
			if args.IP != "" {
				if err := blockIP(args.IP); err != nil {
					return nil, nil, fmt.Errorf("failed to block IP: %w", err)
				}
				return nil, map[string]any{"success": true, "blocked": "ip", "value": args.IP}, nil
			}
			if args.Port != 0 {
				if err := blockPort(args.Port); err != nil {
					return nil, nil, fmt.Errorf("failed to block port: %w", err)
				}
				return nil, map[string]any{"success": true, "blocked": "port", "value": args.Port}, nil
			}
			return nil, nil, fmt.Errorf("must specify either ip or port")
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "block_process_cgroup",
			Title:       "Block Process Cgroup",
			Description: "Block all network traffic from the cgroup of the specified PID. Requires policyManagementEnabled runtime flag.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPBlockCgroupInput) (*mcp.CallToolResult, map[string]any, error) {
			settings := runtimeSettingsStore.Snapshot()
			if !settings.PolicyManagementEnabled {
				return nil, nil, fmt.Errorf("policy management is disabled in runtime settings")
			}
			snap := currentCgroupSandboxSnapshot()
			cgroupID, cgroupPath, err := cgroupIDForPID(int(args.PID), snap.CgroupPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get cgroup for PID: %w", err)
			}
			if err := blockCgroup(cgroupID); err != nil {
				return nil, nil, fmt.Errorf("failed to block cgroup: %w", err)
			}
			return nil, map[string]any{"success": true, "blocked": "cgroup", "pid": args.PID, "cgroupId": cgroupID, "cgroupPath": cgroupPath}, nil
		})

		mcp.AddTool(server, &mcp.Tool{
			Name:        "block_file_access",
			Title:       "Block File Access",
			Description: "Use BPF LSM to block file operations by exact executable path or file/directory basename. Requires policyManagementEnabled runtime flag.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MCPBlockFileInput) (*mcp.CallToolResult, map[string]any, error) {
			settings := runtimeSettingsStore.Snapshot()
			if !settings.PolicyManagementEnabled {
				return nil, nil, fmt.Errorf("policy management is disabled in runtime settings")
			}
			if args.Path != "" && args.IsExec {
				if err := blockLsmExecPath(args.Path); err != nil {
					return nil, nil, fmt.Errorf("failed to block exec path: %w", err)
				}
				return nil, map[string]any{"success": true, "blocked": "exec_path", "value": args.Path}, nil
			}
			if args.Basename != "" {
				if args.IsExec {
					if err := blockLsmExecName(args.Basename); err != nil {
						return nil, nil, fmt.Errorf("failed to block exec name: %w", err)
					}
					return nil, map[string]any{"success": true, "blocked": "exec_name", "value": args.Basename}, nil
				}
				if err := blockLsmFileName(args.Basename); err != nil {
					return nil, nil, fmt.Errorf("failed to block file name: %w", err)
				}
				return nil, map[string]any{"success": true, "blocked": "file_name", "value": args.Basename}, nil
			}
			return nil, nil, fmt.Errorf("must specify either path or basename")
		})

		mcpServer = server
	})

	return mcpServer
}

func buildMCPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return buildMCPServer()
	}, nil)
}
