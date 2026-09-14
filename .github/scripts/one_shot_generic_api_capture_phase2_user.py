from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing phase2 userspace anchor in {path}: {old[:160]!r}")
    write(path, text.replace(old, new, 1))


# ---------------------------------------------------------------------------
# captureprofile: source/direction selectors, overlay hot reload, HTTP start
# lines and generic gRPC route normalization.
# ---------------------------------------------------------------------------
replace_once(
    "backend/app/captureprofile/profile.go",
    '''import (\n\t"encoding/json"\n\t"fmt"\n\t"net/url"\n\t"os"\n\t"sort"\n\t"strings"\n\t"sync/atomic"\n)''',
    '''import (\n\t"context"\n\t"crypto/sha256"\n\t"encoding/json"\n\t"fmt"\n\t"net/url"\n\t"os"\n\t"sort"\n\t"strconv"\n\t"strings"\n\t"sync/atomic"\n\t"time"\n)''')

replace_once(
    "backend/app/captureprofile/profile.go",
    '''\tOperation       string   `json:"operation,omitempty"`\n\tProtocols       []string `json:"protocols,omitempty"`\n\tMethods         []string `json:"methods,omitempty"`\n''',
    '''\tOperation       string   `json:"operation,omitempty"`\n\tSources         []string `json:"sources,omitempty"`\n\tProtocols       []string `json:"protocols,omitempty"`\n\tDirections      []string `json:"directions,omitempty"`\n\tMethods         []string `json:"methods,omitempty"`\n''')

replace_once(
    "backend/app/captureprofile/profile.go",
    '''\tprofile.Operation = strings.TrimSpace(profile.Operation)\n\tprofile.Protocols = normalizeList(profile.Protocols)\n\tprofile.Methods = normalizeUpperList(profile.Methods)\n''',
    '''\tprofile.Operation = strings.TrimSpace(profile.Operation)\n\tprofile.Sources = normalizeList(profile.Sources)\n\tprofile.Protocols = normalizeList(profile.Protocols)\n\tprofile.Directions = normalizeList(profile.Directions)\n\tprofile.Methods = normalizeUpperList(profile.Methods)\n''')

replace_once(
    "backend/app/captureprofile/profile.go",
    '''\tscore := 0\n\tmatchedBy := make([]string, 0, 6)\n\n\tif len(profile.Protocols) != 0 {\n''',
    '''\tscore := 0\n\tmatchedBy := make([]string, 0, 8)\n\n\tif len(profile.Sources) != 0 {\n\t\tif !containsFold(profile.Sources, observation.Source) {\n\t\t\treturn Match{}, false\n\t\t}\n\t\tscore += 3\n\t\tmatchedBy = append(matchedBy, "source")\n\t}\n\tif len(profile.Protocols) != 0 {\n''')
replace_once(
    "backend/app/captureprofile/profile.go",
    '''\tif len(profile.Methods) != 0 {\n''',
    '''\tif len(profile.Directions) != 0 {\n\t\tif !containsFold(profile.Directions, observation.Direction) {\n\t\t\treturn Match{}, false\n\t\t}\n\t\tscore += 3\n\t\tmatchedBy = append(matchedBy, "direction")\n\t}\n\tif len(profile.Methods) != 0 {\n''')

# Replace request-line parser with generic request/response start-line parser,
# preserving the old helper as a compatibility wrapper.
old_parser = '''func ParseHTTP1RequestLine(line string) (method, path string, ok bool) {\n\tline = strings.TrimSpace(strings.TrimRight(line, "\\x00"))\n\tif line == "" {\n\t\treturn "", "", false\n\t}\n\tif index := strings.IndexAny(line, "\\r\\n"); index >= 0 {\n\t\tline = line[:index]\n\t}\n\tparts := strings.Fields(line)\n\tif len(parts) < 2 {\n\t\treturn "", "", false\n\t}\n\tmethod = strings.ToUpper(parts[0])\n\tif !isHTTPMethod(method) {\n\t\treturn "", "", false\n\t}\n\tpath = RequestPath(parts[1])\n\tif path == "" {\n\t\treturn "", "", false\n\t}\n\treturn method, path, true\n}\n'''
new_parser = '''type HTTP1StartLine struct {\n\tKind   string\n\tMethod string\n\tPath   string\n\tStatus uint32\n}\n\nfunc ParseHTTP1StartLine(line string) (HTTP1StartLine, bool) {\n\tline = strings.TrimSpace(strings.TrimRight(line, "\\x00"))\n\tif line == "" {\n\t\treturn HTTP1StartLine{}, false\n\t}\n\tif index := strings.IndexAny(line, "\\r\\n"); index >= 0 {\n\t\tline = line[:index]\n\t}\n\tparts := strings.Fields(line)\n\tif len(parts) < 2 {\n\t\treturn HTTP1StartLine{}, false\n\t}\n\tif strings.HasPrefix(strings.ToUpper(parts[0]), "HTTP/1.") {\n\t\tstatus, err := strconv.ParseUint(parts[1], 10, 32)\n\t\tif err != nil || status < 100 || status > 999 {\n\t\t\treturn HTTP1StartLine{}, false\n\t\t}\n\t\treturn HTTP1StartLine{Kind: "response", Status: uint32(status)}, true\n\t}\n\tmethod := strings.ToUpper(parts[0])\n\tif !isHTTPMethod(method) {\n\t\treturn HTTP1StartLine{}, false\n\t}\n\tpath := RequestPath(parts[1])\n\tif path == "" {\n\t\treturn HTTP1StartLine{}, false\n\t}\n\treturn HTTP1StartLine{Kind: "request", Method: method, Path: path}, true\n}\n\nfunc ParseHTTP1RequestLine(line string) (method, path string, ok bool) {\n\tparsed, ok := ParseHTTP1StartLine(line)\n\tif !ok || parsed.Kind != "request" {\n\t\treturn "", "", false\n\t}\n\treturn parsed.Method, parsed.Path, true\n}\n\n// ParseGRPCPath normalizes the canonical HTTP/2 gRPC route\n// /package.Service/Method without inspecting protobuf message bodies.\nfunc ParseGRPCPath(raw string) (service, method string, ok bool) {\n\tpath := strings.TrimPrefix(RequestPath(raw), "/")\n\tparts := strings.Split(path, "/")\n\tif len(parts) != 2 {\n\t\treturn "", "", false\n\t}\n\tservice = strings.TrimSpace(parts[0])\n\tmethod = strings.TrimSpace(parts[1])\n\tif service == "" || method == "" {\n\t\treturn "", "", false\n\t}\n\treturn service, method, true\n}\n'''
replace_once("backend/app/captureprofile/profile.go", old_parser, new_parser)

# Overlay + reload worker before BuiltinProfiles.
replace_once(
    "backend/app/captureprofile/profile.go",
    '''func BuiltinProfiles() []Profile {\n''',
    r'''// MergeProfiles overlays custom profiles by ID while retaining all built-ins
// that were not explicitly replaced. The result is validated atomically by the
// Registry before publication.
func MergeProfiles(base, overlay []Profile) []Profile {
	merged := make(map[string]Profile, len(base)+len(overlay))
	for _, profile := range base {
		profile = normalizeProfile(profile)
		if profile.ID != "" {
			merged[profile.ID] = profile
		}
	}
	for _, profile := range overlay {
		profile = normalizeProfile(profile)
		if profile.ID != "" {
			merged[profile.ID] = profile
		}
	}
	out := make([]Profile, 0, len(merged))
	for _, profile := range merged {
		out = append(out, profile)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ReloadDefaultJSON(path string) error {
	overlay, err := LoadJSON(path)
	if err != nil {
		return err
	}
	return Default.Replace(MergeProfiles(BuiltinProfiles(), overlay))
}

// WatchDefaultJSON hashes the file contents instead of relying only on mtime,
// which makes atomic replace/ConfigMap style updates reliable. Invalid updates
// never replace the last known-good immutable registry snapshot.
func WatchDefaultJSON(ctx context.Context, path string, interval time.Duration, onError func(error)) {
	if ctx == nil || strings.TrimSpace(path) == "" {
		return
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	var lastAttempt [32]byte
	var attempted bool
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(path)
			if err != nil {
				if onError != nil {
					onError(fmt.Errorf("read API capture profiles: %w", err))
				}
				continue
			}
			digest := sha256.Sum256(data)
			if attempted && digest == lastAttempt {
				continue
			}
			lastAttempt = digest
			attempted = true
			overlay, err := ParseJSON(data)
			if err == nil {
				err = Default.Replace(MergeProfiles(BuiltinProfiles(), overlay))
			}
			if err != nil && onError != nil {
				onError(err)
			}
		}
	}
}

func BuiltinProfiles() []Profile {
''')

# Tests for response line, gRPC route, source/direction and overlay override.
path = "backend/app/captureprofile/profile_test.go"
text = read(path)
text += r'''

func TestParseHTTP1ResponseLine(t *testing.T) {
	line, ok := ParseHTTP1StartLine("HTTP/1.1 429 Too Many Requests\r\n")
	if !ok || line.Kind != "response" || line.Status != 429 {
		t.Fatalf("response line = %+v ok=%v", line, ok)
	}
}

func TestParseGRPCPath(t *testing.T) {
	service, method, ok := ParseGRPCPath("/google.ai.generativelanguage.v1beta.GenerativeService/GenerateContent?key=secret")
	if !ok || service != "google.ai.generativelanguage.v1beta.GenerativeService" || method != "GenerateContent" {
		t.Fatalf("grpc service=%q method=%q ok=%v", service, method, ok)
	}
}

func TestProfileSourceAndDirectionSelectors(t *testing.T) {
	registry := NewRegistry([]Profile{{
		ID: "acme.inbound", Vendor: "acme", Sources: []string{"kernel_socket_prefix"},
		Protocols: []string{"http1"}, Directions: []string{"incoming"}, Methods: []string{"POST"},
		PathPrefixes: []string{"/hook"}, MinScore: 40,
	}})
	if _, ok := registry.Match(Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Path: "/hook"}); ok {
		t.Fatal("outgoing observation unexpectedly matched inbound profile")
	}
	if match, ok := registry.Match(Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "incoming", Method: "POST", Path: "/hook"}); !ok || match.ProfileID != "acme.inbound" {
		t.Fatalf("inbound match failed: ok=%v match=%+v", ok, match)
	}
}

func TestMergeProfilesCustomOverridesBuiltinID(t *testing.T) {
	merged := MergeProfiles(BuiltinProfiles(), []Profile{{
		ID: "openai.responses", Vendor: "private-gateway", Product: "responses", Operation: "proxy.responses",
		Protocols: []string{"http2"}, HostSuffixes: []string{"gateway.example"}, PathPrefixes: []string{"/v1/responses"}, MinScore: 90,
	}})
	registry := NewRegistry(merged)
	match, ok := registry.Match(Observation{Protocol: "http2", Method: "POST", Host: "gateway.example", Path: "/v1/responses"})
	if !ok || match.Vendor != "private-gateway" || match.Operation != "proxy.responses" {
		t.Fatalf("overlay override failed: ok=%v match=%+v", ok, match)
	}
}
'''
write(path, text)

# ---------------------------------------------------------------------------
# TLS protocol normalization: recognize gRPC from HTTP/2 content-type and
# provide generic service/method identity even without a vendor profile.
# ---------------------------------------------------------------------------
replace_once(
    "backend/app/tls/api_fingerprint.go",
    '''\tif strings.HasPrefix(event.Type, "http2_") {\n\t\treturn "http2"\n\t}\n''',
    '''\tif strings.HasPrefix(event.Type, "http2_") {\n\t\tcontentType := strings.ToLower(strings.TrimSpace(event.ContentType))\n\t\tif contentType == "" {\n\t\t\tfor key, value := range event.Headers {\n\t\t\t\tif strings.EqualFold(strings.TrimSpace(key), "content-type") {\n\t\t\t\t\tcontentType = strings.ToLower(strings.TrimSpace(value))\n\t\t\t\t\tbreak\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t\tif strings.Contains(contentType, "application/grpc") {\n\t\t\treturn "grpc"\n\t\t}\n\t\treturn "http2"\n\t}\n''')

old_match_tail = '''\tmatch, ok := captureprofile.Default.Match(captureprofile.Observation{\n\t\tSource:      event.CaptureSource,\n\t\tProtocol:    protocol,\n\t\tDirection:   event.Direction,\n\t\tMethod:      event.Method,\n\t\tHost:        event.Host,\n\t\tPath:        event.RequestPath,\n\t\tHeaders:     event.Headers,\n\t\tContentType: event.ContentType,\n\t})\n\tif !ok {\n\t\treturn\n\t}\n\tevent.APIProfile = match.ProfileID\n\tevent.APIProduct = match.Product\n\tevent.APIOperation = match.Operation\n\tevent.APIConfidence = match.Confidence\n\tif event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {\n\t\tevent.Vendor = match.Vendor\n\t}\n}\n'''
new_match_tail = '''\tmatch, ok := captureprofile.Default.Match(captureprofile.Observation{\n\t\tSource:      event.CaptureSource,\n\t\tProtocol:    protocol,\n\t\tDirection:   event.Direction,\n\t\tMethod:      event.Method,\n\t\tHost:        event.Host,\n\t\tPath:        event.RequestPath,\n\t\tHeaders:     event.Headers,\n\t\tContentType: event.ContentType,\n\t})\n\tif ok {\n\t\tevent.APIProfile = match.ProfileID\n\t\tevent.APIProduct = match.Product\n\t\tevent.APIOperation = match.Operation\n\t\tevent.APIConfidence = match.Confidence\n\t\tif event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {\n\t\t\tevent.Vendor = match.Vendor\n\t\t}\n\t\treturn\n\t}\n\tif protocol == "grpc" {\n\t\tif service, method, parsed := captureprofile.ParseGRPCPath(event.RequestPath); parsed {\n\t\t\tevent.APIProfile = "grpc:" + service\n\t\t\tevent.APIProduct = service\n\t\t\tevent.APIOperation = method\n\t\t\tevent.APIConfidence = 70\n\t\t}\n\t}\n}\n'''
replace_once("backend/app/tls/api_fingerprint.go", old_match_tail, new_match_tail)

path = "backend/app/tls/api_fingerprint_test.go"
text = read(path)
text += r'''

func TestAnnotateTLSAPIFingerprintGRPC(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http2_headers", Direction: "send", Method: "POST",
		Host: "grpc.example", URL: "/acme.agent.v1.AgentService/Run?token=secret",
		ContentType: "application/grpc+proto",
	}
	annotateTLSAPIFingerprint(event)
	if event.AppProtocol != "grpc" || event.RequestPath != "/acme.agent.v1.AgentService/Run" {
		t.Fatalf("grpc normalization = %+v", event)
	}
	if event.APIProduct != "acme.agent.v1.AgentService" || event.APIOperation != "Run" || event.APIConfidence != 70 {
		t.Fatalf("grpc fingerprint = %+v", event)
	}
}
'''
write(path, text)

# ---------------------------------------------------------------------------
# Kernel event normalization consumes raw flags rather than guessing source.
# ---------------------------------------------------------------------------
replace_once(
    "backend/app/events/events_network.go",
    '''// ── Pure helper functions (no external deps needed) ────────────────────\n''',
    '''// ── Pure helper functions (no external deps needed) ────────────────────\n\nconst (\n\tkernelCaptureHTTPRequest uint32 = 1 << iota\n\tkernelCaptureHTTPResponse\n\tkernelCaptureIncoming\n\tkernelCaptureOutgoing\n\tkernelCaptureFDDuplicated\n\tkernelCaptureFDInherited\n\tkernelCaptureAccepted\n\tkernelCaptureScatterGather\n)\n''')

old_socket_case = '''\tcase "socket_http":\n\t\tmethod, requestPath, ok := captureprofile.ParseHTTP1RequestLine(extraPath)\n\t\tout.CaptureSource = "kernel_socket_prefix"\n\t\tout.AppProtocol = "http1"\n\t\tout.KernelSocketFd = int32(event.Extra1)\n\t\tout.KernelPayloadPrefixLen = event.Extra2\n\t\tout.KernelCaptureFlags = 1\n\t\tout.Bytes = event.Extra3\n\t\tif ok {\n\t\t\tout.HttpMethod = method\n\t\t\tout.HttpPath = requestPath\n\t\t\t// Drop the raw request line immediately so query parameters never\n\t\t\t// leave the kernel-event normalization boundary.\n\t\t\tout.ExtraPath = method + " " + requestPath\n\t\t\tif match, matched := captureprofile.Default.Match(captureprofile.Observation{\n\t\t\t\tSource: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing",\n\t\t\t\tMethod: method, Path: requestPath,\n\t\t\t}); matched {\n\t\t\t\tout.ApiProfile = match.ProfileID\n\t\t\t\tout.ApiVendor = match.Vendor\n\t\t\t\tout.ApiProduct = match.Product\n\t\t\t\tout.ApiOperation = match.Operation\n\t\t\t\tout.ApiConfidence = match.Confidence\n\t\t\t\tout.ServiceName = match.Vendor\n\t\t\t}\n\t\t}\n\t\tout.ExtraInfo = fmt.Sprintf("fd=%d capture=request-line prefix_len=%d requested=%d", int32(event.Extra1), event.Extra2, event.Extra3)\n'''
new_socket_case = '''\tcase "socket_http":\n\t\tstartLine, ok := captureprofile.ParseHTTP1StartLine(extraPath)\n\t\tout.CaptureSource = "kernel_socket_prefix"\n\t\tout.AppProtocol = "http1"\n\t\tout.KernelSocketFd = int32(event.Extra1)\n\t\tout.KernelPayloadPrefixLen = event.Extra2\n\t\tout.KernelCaptureFlags = event.KernelCaptureFlags\n\t\tout.Bytes = event.Extra3\n\t\tdirection := "outgoing"\n\t\tif event.KernelCaptureFlags&kernelCaptureIncoming != 0 {\n\t\t\tdirection = "incoming"\n\t\t}\n\t\tif ok && startLine.Kind == "request" {\n\t\t\tout.HttpMethod = startLine.Method\n\t\t\tout.HttpPath = startLine.Path\n\t\t\tout.ExtraPath = startLine.Method + " " + startLine.Path\n\t\t\tif match, matched := captureprofile.Default.Match(captureprofile.Observation{\n\t\t\t\tSource: "kernel_socket_prefix", Protocol: "http1", Direction: direction,\n\t\t\t\tMethod: startLine.Method, Path: startLine.Path,\n\t\t\t}); matched {\n\t\t\t\tout.ApiProfile = match.ProfileID\n\t\t\t\tout.ApiVendor = match.Vendor\n\t\t\t\tout.ApiProduct = match.Product\n\t\t\t\tout.ApiOperation = match.Operation\n\t\t\t\tout.ApiConfidence = match.Confidence\n\t\t\t\tout.ServiceName = match.Vendor\n\t\t\t}\n\t\t} else if ok && startLine.Kind == "response" {\n\t\t\tout.HttpStatus = startLine.Status\n\t\t\tout.ExtraPath = fmt.Sprintf("HTTP status %d", startLine.Status)\n\t\t}\n\t\tkind := "start-line"\n\t\tif event.KernelCaptureFlags&kernelCaptureHTTPRequest != 0 {\n\t\t\tkind = "request-line"\n\t\t} else if event.KernelCaptureFlags&kernelCaptureHTTPResponse != 0 {\n\t\t\tkind = "response-line"\n\t\t}\n\t\tout.ExtraInfo = fmt.Sprintf("fd=%d capture=%s direction=%s prefix_len=%d bytes=%d flags=0x%x", int32(event.Extra1), kind, direction, event.Extra2, event.Extra3, event.KernelCaptureFlags)\n'''
replace_once("backend/app/events/events_network.go", old_socket_case, new_socket_case)

# ---------------------------------------------------------------------------
# Default profile file hot reload worker. Empty env means zero overhead.
# ---------------------------------------------------------------------------
replace_once(
    "backend/app/jobs_background.go",
    '''import (\n\t"agent-ebpf-filter/app/recording"\n''',
    '''import (\n\t"agent-ebpf-filter/app/captureprofile"\n\t"agent-ebpf-filter/app/recording"\n''')
replace_once(
    "backend/app/jobs_background.go",
    '''\t"os"\n\t"sync"\n''',
    '''\t"os"\n\t"strings"\n\t"sync"\n''')
replace_once(
    "backend/app/jobs_background.go",
    '''\tjobs := &runtimeBackgroundJobs{}\n\tinitRedactionEngine()\n''',
    '''\tjobs := &runtimeBackgroundJobs{}\n\tinitRedactionEngine()\n\tstartAPICaptureProfileWatcher(ctx, jobs)\n''')
replace_once(
    "backend/app/jobs_background.go",
    '''func runSemanticAlertStateGC(ctx context.Context, state *events.SemanticAlertState, interval time.Duration) {\n''',
    r'''func startAPICaptureProfileWatcher(ctx context.Context, jobs *runtimeBackgroundJobs) {
	if ctx == nil || jobs == nil {
		return
	}
	path := strings.TrimSpace(os.Getenv("AGENT_EBPF_API_PROFILES"))
	if path == "" {
		return
	}
	if err := captureprofile.ReloadDefaultJSON(path); err != nil {
		log.Printf("[WARN] initial API capture profile load failed: %v", err)
	} else {
		log.Printf("[INFO] API capture profiles loaded from %s", path)
	}
	jobs.Go(func() {
		captureprofile.WatchDefaultJSON(ctx, path, 2*time.Second, func(err error) {
			log.Printf("[WARN] API capture profile reload rejected; keeping last known-good rules: %v", err)
		})
	})
}

func runSemanticAlertStateGC(ctx context.Context, state *events.SemanticAlertState, interval time.Duration) {
''')

# Documentation: phase2 capability contract and runtime overlay.
path = "docs/backend/generic-api-capture.md"
text = read(path)
text += r'''

## Phase 2: descriptor lineage and bidirectional metadata

The kernel socket identity layer now follows descriptor topology rather than only the original `socket()` return value:

- `dup`, `dup2`, and `dup3` copy or invalidate socket provenance as descriptors are replaced;
- `sched_process_fork` records a bounded lazy parent lineage, allowing a child to materialize inherited socket metadata on first use without iterating descriptor tables in eBPF;
- `accept` and `accept4` mark returned sockets as accepted and preserve listener family/type/protocol plus peer endpoint when the sockaddr is available;
- `read` and `write` recognize both HTTP/1 request lines and response status lines on confirmed plaintext sockets;
- raw `kernel_capture_flags` distinguish request/response, incoming/outgoing, duplicated/inherited/accepted descriptor provenance, and future scatter/gather capture.

The lineage resolver intentionally follows at most two parent generations before materializing a child entry. This keeps verifier complexity and map lookups bounded. Long-lived descendants normally materialize entries during their first socket operation, making subsequent lookups direct.

## Runtime profile overlays

Set `AGENT_EBPF_API_PROFILES=/path/to/profiles.json` to layer custom rules over the built-ins. The file remains a JSON array of profile objects. Custom profiles with an existing `id` replace that built-in ID; new IDs are appended. The watcher hashes file contents every two seconds and publishes a fully validated immutable snapshot atomically. Invalid updates are rejected while the last known-good rules remain active.

Profiles can additionally restrict `sources` and `directions`, for example `kernel_socket_prefix` + `incoming` for a local webhook/API server. This lets the same engine classify client APIs, reverse proxies, local gateways, and self-hosted OpenAI/Anthropic-compatible endpoints without adding provider-specific eBPF code.

## gRPC

HTTP/2 plaintext events with an `application/grpc` content type are normalized to `grpc`. The canonical `/package.Service/Method` route is split into service and method without decoding protobuf bodies. If no explicit vendor profile matches, the event still receives a generic `grpc:<service>` profile, service as the product, method as the operation, and a moderate metadata-only confidence score.
'''
write(path, text)
