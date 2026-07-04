package handlers

import (
	"net/http"
	"strings"
	"time"

	"agent-ebpf-filter/app/platform"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
)

// ---- moved from app/api_external.go ----

// HandleExternalAPIHealth returns service health, collector counters, feature
// gates, and eBPF bootstrap status.
func HandleExternalAPIHealth(c *gin.Context) {
	settings := Deps.RuntimeSettings.Snapshot()
	manifest := Deps.BuildFeatureManifest(settings).(FeatureManifestWrapper)
	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"service":    "agent-ebpf-filter",
		"apiVersion": "v1",
		"port":       platform.ResolveBackendPort(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"features": gin.H{
			"logPersistenceEnabled":     settings.LogPersistenceEnabled,
			"shellSessionsEnabled":      settings.ShellSessionsEnabled,
			"systemRunEnabled":          settings.SystemRunEnabled,
			"hookManagementEnabled":     settings.HookManagementEnabled,
			"policyManagementEnabled":   settings.PolicyManagementEnabled,
			"kernelRiskFeedbackEnabled": settings.KernelRiskFeedback.Enabled,
			"otlpEnabled":               settings.OtlpEnabled,
			"tlsCaptureEnabled":         settings.TlsCaptureEnabled,
			"domainForwardProxyEnabled": settings.DomainForwardProxy.Enabled,
		},
		"featureManifest": manifest.Features,
		"bootstrap":       Deps.BootstrapTracepointStatus(),
		"collector":       Deps.CollectorHealth(),
	})
}

// FeatureManifestWrapper is a minimal interface for the feature manifest response.
type FeatureManifestWrapper struct {
	Features any `json:"features"`
}

// HandleExternalAPIOpenAPI returns the external API OpenAPI spec.
func HandleExternalAPIOpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, BuildExternalOpenAPISpec())
}

// BuildExternalOpenAPISpec builds the external API description using the
// kin-openapi typed model (github.com/getkin/kin-openapi/openapi3) so the
// document is type-checked at compile time and can be validated with
// (*openapi3.T).Validate. *openapi3.T marshals to a standards-compliant
// OpenAPI 3.0.3 JSON document via its json.Marshaler implementation.
func BuildExternalOpenAPISpec() *openapi3.T {
	paths := openapi3.NewPaths()

	addOperation(paths, "/health", http.MethodGet, "Service health, collector counters, feature gates, and eBPF bootstrap status.")
	addOperation(paths, "/events/recent", http.MethodGet, "Recent captured events. Query: limit, type, event_type, source, pid, comm, trace_id, span_id, since, until, redaction_state.")
	addOperation(paths, "/events/graph", http.MethodGet, "Aggregated execution graph. Query: agent_run_id, tool_call_id, trace_id, pid, path, domain, risk_min, since, until.")
	addOperation(paths, "/research/sessions", http.MethodGet, "List persisted research sessions and summaries.")
	addOperation(paths, "/research/sessions", http.MethodPost, "Create a research session with optional source filters, time range, tags, and description.")
	addOperation(paths, "/research/sessions/{id}", http.MethodGet, "Get research session metadata, summary, and artifact references.")
	addOperation(paths, "/research/sessions/{id}", http.MethodDelete, "Delete a research session and its artifacts.")
	addOperation(paths, "/research/sessions/{id}/tasks", http.MethodPost, "Queue an asynchronous research task: scan_recent, build_session, compare_windows, export_bundle, reset_session.")
	addOperation(paths, "/research/tasks/{taskId}", http.MethodGet, "Get asynchronous research task status, progress, result reference, or error.")
	addOperation(paths, "/research/tasks/{taskId}/cancel", http.MethodPost, "Cancel a queued or running research task idempotently.")
	addOperation(paths, "/research/sessions/{id}/events", http.MethodGet, "Page normalized research events. Query: limit, offset, source, eventType, pid, comm, traceId, spanId, since, until, q.")
	addOperation(paths, "/research/sessions/{id}/results", http.MethodGet, "Get backend-computed research aggregates, process/trace/timeline summaries, loop findings, and risk alerts.")
	addOperation(paths, "/research/sessions/{id}/training", http.MethodGet, "Export a training-ready structured dataset with 128-dim normalized feature vectors. Query: format=json|jsonl|csv, labelPolicy=heuristic|decision|unlabeled.")
	addOperation(paths, "/research/sessions/{id}/training/import", http.MethodPost, "Import labeled research training samples into the ML training store. Body: {\"labelPolicy\":\"decision\",\"limit\":500,\"preview\":false}.")
	addOperation(paths, "/research/sessions/{id}/export", http.MethodGet, "Download generated research artifact. Query: format=jsonl|csv|bundle.")
	addOperation(paths, "/agentsight/events", http.MethodGet, "AgentSight-compatible merged event export. Query: limit, format=json|array|jsonl, include_tls, source, pid, comm, event_type, type, trace_id, span_id, since, until, filter.")
	addOperation(paths, "/agentsight/events", http.MethodPost, "Import AgentSight JSON, JSON arrays, {\"events\":[...]}, or JSONL text into the compatibility store.")
	addOperation(paths, "/agentsight/events.jsonl", http.MethodGet, "AgentSight-compatible JSONL export of merged retained events and TLS capture history.")
	addOperation(paths, "/agentsight/events/stats", http.MethodGet, "AgentSight storage statistics by semantic source, event type, runner, and command.")
	addOperation(paths, "/agentsight/events/runners/{id}/stats", http.MethodGet, "AgentSight event statistics for one logical runner.")
	addOperation(paths, "/agentsight/events/query", http.MethodPost, "Advanced AgentSight query with JSON body filters for sources, event types, PIDs, runner, time range, and text search.")
	addOperation(paths, "/agentsight/events/stream", http.MethodGet, "Server-sent AgentSight-compatible stream for clients that cannot use WebSockets.")
	addOperation(paths, "/agentsight/runners", http.MethodGet, "List AgentSight logical runners and their status.")
	addOperation(paths, "/agentsight/stream/merged", http.MethodGet, "Server-sent AgentSight-compatible merged stream from all logical runners.")
	addOperation(paths, "/agentsight/stream/runner/{id}", http.MethodGet, "Server-sent AgentSight-compatible stream filtered to one logical runner.")
	addOperation(paths, "/network/flows", http.MethodGet, "Attributed TCP/UDP flow summaries. Query: filter, sort, showHistoric, limit, cursor, pid, domain, service, scope.")
	addOperation(paths, "/network/flows/{flowID}", http.MethodGet, "One network flow by stable flow id.")
	addOperation(paths, "/network/dns-cache", http.MethodGet, "DNS correlation cache.")
	addOperation(paths, "/network/interfaces", http.MethodGet, "Per-interface RX/TX counters, packets, errors, and drops.")
	addOperation(paths, "/network/export/jsonl", http.MethodGet, "Metadata-only flow JSONL export.")
	addOperation(paths, "/sandbox/cgroup/status", http.MethodGet, "cgroup/connect + sendmsg enforcement status, maps, counters, and active blocks.")
	addOperation(paths, "/sandbox/lsm/status", http.MethodGet, "BPF LSM enforcement status, maps, counters, and active blocks.")
	addOperation(paths, "/policies/network/block-ip", http.MethodPost, "Block an IPv4/IPv6 destination through the cgroup sandbox. Body: {\"ip\":\"203.0.113.10\"}.")
	addOperation(paths, "/policies/network/unblock-ip", http.MethodPost, "Remove an IP destination block. Body: {\"ip\":\"203.0.113.10\"}.")
	addOperation(paths, "/policies/network/block-port", http.MethodPost, "Block a TCP/UDP destination port. Body: {\"port\":4444}.")
	addOperation(paths, "/policies/network/unblock-port", http.MethodPost, "Remove a TCP/UDP destination-port block. Body: {\"port\":4444}.")
	addOperation(paths, "/policies/network/block-pid", http.MethodPost, "Resolve a PID's cgroup v2 inode id and block outbound networking for that cgroup. Body: {\"pid\":1234}.")
	addOperation(paths, "/policies/network/unblock-pid", http.MethodPost, "Resolve a PID's cgroup v2 inode id and remove its cgroup network block. Body: {\"pid\":1234}.")
	addOperation(paths, "/policies/lsm/block-exec-path", http.MethodPost, "Block an executable path with BPF LSM. Body: {\"path\":\"/usr/bin/nc\"}.")
	addOperation(paths, "/policies/lsm/unblock-exec-path", http.MethodPost, "Remove an executable-path block. Body: {\"path\":\"/usr/bin/nc\"}.")
	addOperation(paths, "/policies/lsm/block-exec-name", http.MethodPost, "Block executable basename with BPF LSM. Body: {\"name\":\"nc\"}.")
	addOperation(paths, "/policies/lsm/unblock-exec-name", http.MethodPost, "Remove executable-basename block. Body: {\"name\":\"nc\"}.")
	addOperation(paths, "/policies/lsm/block-file-name", http.MethodPost, "Block file/directory basename for open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename. Body: {\"name\":\"id_rsa\"}.")
	addOperation(paths, "/policies/lsm/unblock-file-name", http.MethodPost, "Remove file/directory basename block. Body: {\"name\":\"id_rsa\"}.")
	addOperation(paths, "/agents/register", http.MethodPost, "Register an agent PID and optional run/task/tool context. Body follows the root /register payload.")
	addOperation(paths, "/agents/unregister", http.MethodPost, "Unregister an agent PID. Body: {\"pid\":1234}.")
	addOperation(paths, "/config/export", http.MethodGet, "Export tags, tracked commands, tracked paths, wrapper rules, and runtime settings.")

	return &openapi3.T{
		OpenAPI: "3.0.3",
		Info: &openapi3.Info{
			Title:       "Agent eBPF Filter external API",
			Version:     "v1",
			Description: "Stable aliases for external automation, observability collectors, and Kubernetes in-cluster callers.",
		},
		Servers: openapi3.Servers{{URL: "/api/v1"}},
		Components: &openapi3.Components{
			SecuritySchemes: openapi3.SecuritySchemes{
				"ApiKeyAuth": &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
					Type: "apiKey", In: "header", Name: "X-API-KEY",
				}},
				"BearerAuth": &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
					Type: "http", Scheme: "bearer",
				}},
				"QueryKey": &openapi3.SecuritySchemeRef{Value: &openapi3.SecurityScheme{
					Type: "apiKey", In: "query", Name: "key",
					Description: "Use only for WebSocket/SSE clients or tightly controlled automation where headers are unavailable.",
				}},
			},
		},
		Security: openapi3.SecurityRequirements{
			{"ApiKeyAuth": []string{}},
			{"BearerAuth": []string{}},
			{"QueryKey": []string{}},
		},
		Paths: paths,
	}
}

// addOperation attaches a single-response (200 OK) operation for method on path,
// creating the path item if it does not yet exist (so a path can carry both GET
// and POST, as /agentsight/events does). Any {name} placeholders in the path are
// declared as required string path parameters so the document passes validation.
func addOperation(paths *openapi3.Paths, path, method, summary string) {
	item := paths.Find(path)
	if item == nil {
		item = &openapi3.PathItem{}
		paths.Set(path, item)
	}
	op := &openapi3.Operation{
		Summary: summary,
		Responses: openapi3.NewResponses(openapi3.WithStatus(http.StatusOK,
			&openapi3.ResponseRef{Value: openapi3.NewResponse().WithDescription("OK")})),
	}
	for _, name := range pathParamNames(path) {
		param := openapi3.NewPathParameter(name)
		param.Schema = openapi3.NewStringSchema().NewRef()
		op.AddParameter(param)
	}
	item.SetOperation(method, op)
}

// pathParamNames returns the names inside {…} placeholders of an OpenAPI path.
func pathParamNames(path string) []string {
	var names []string
	for {
		start := strings.IndexByte(path, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(path[start:], '}')
		if end < 0 {
			break
		}
		names = append(names, path[start+1:start+end])
		path = path[start+end+1:]
	}
	return names
}
