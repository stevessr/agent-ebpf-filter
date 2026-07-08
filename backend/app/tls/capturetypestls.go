package tls

import (
	"time"
)

// ---- moved from backend/zz_merged_backend.go section capturetypestls.go ----

const tlsFragmentSize = 960
const tlsMaxFragments = 18

const tlsLibOpenSSL = 0
const tlsLibGo = 1
const tlsLibGnuTLS = 2
const tlsLibNSS = 3
const tlsLibRustls = 4 // rustls (Rust static-pie binaries: codex, cursor) — captured via offset uprobe

const tlsDirectionRecv = 0
const tlsDirectionSend = 1

const tlsFlagTruncated = 1

const tlsFuncSSLWrite = 1
const tlsFuncSSLRead = 2
const tlsFuncSSLWriteEx = 3
const tlsFuncSSLReadEx = 4
const tlsFuncGnuTLSRecordSend = 5
const tlsFuncGnuTLSRecordRecv = 6
const tlsFuncPRWrite = 7
const tlsFuncPRRead = 8
const tlsFuncGoConnWrite = 9
const tlsFuncGoConnRead = 10
const tlsFuncSSLWriteEx2 = 11

type tlsFragment struct {
	TimestampNS uint64
	PID         uint32
	TGID        uint32
	DataLen     uint32
	TotalLen    uint32
	OriginalLen uint32
	FragIndex   uint16
	FragCount   uint16
	LibType     uint8
	Direction   uint8
	Flags       uint8
	Function    uint8
	Comm        [16]byte
	Data        [tlsFragmentSize]byte
}

type CompletedTLSFragment struct {
	TimestampNS uint64
	PID         uint32
	TGID        uint32
	DataLen     uint32
	TotalLen    uint32
	OriginalLen uint32
	FragCount   uint16
	LibType     uint8
	Direction   uint8
	Flags       uint8
	Function    uint8
	Comm        string
	Payload     []byte
}

type TLSPlaintextEvent struct {
	Type           string            `json:"type"`
	Timestamp      time.Time         `json:"timestamp"`
	PID            uint32            `json:"pid"`
	TGID           uint32            `json:"tgid"`
	Comm           string            `json:"comm"`
	Direction      string            `json:"direction"`
	Lib            string            `json:"lib"`
	Function       string            `json:"function,omitempty"`
	CapturedLen    int               `json:"captured_len"`
	OriginalLen    int               `json:"original_len"`
	Method         string            `json:"method,omitempty"`
	URL            string            `json:"url,omitempty"`
	Host           string            `json:"host,omitempty"`
	StatusCode     int               `json:"status,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           string            `json:"body,omitempty"`
	BodySize       int               `json:"body_size"`
	ContentType    string            `json:"content_type,omitempty"`
	RawHexDump     string            `json:"raw_hex_dump,omitempty"`
	RawAvailable   bool              `json:"raw_available"`
	Truncated      bool              `json:"truncated"`
	RedactionState string            `json:"redaction_state,omitempty"`
	SSEEvent       string            `json:"sse_event,omitempty"`
	SSEDataDigest  string            `json:"sse_data_digest,omitempty"`
	SSEDataCount   int               `json:"sse_data_count,omitempty"`

	RootAgentPID   uint32 `json:"root_agent_pid,omitempty"`
	AgentRunID     string `json:"agent_run_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	SpanID         string `json:"span_id,omitempty"`

	MessageRole  string `json:"message_role,omitempty"`
	PromptDigest string `json:"prompt_digest,omitempty"`
	PromptLen    int    `json:"prompt_len,omitempty"`
	Vendor       string `json:"vendor,omitempty"`
	LoopAlert    bool   `json:"loop_alert,omitempty"`

	// AgentSight-compatible fields (from sslsniff reference)
	UID         uint32  `json:"uid,omitempty"`
	TID         uint32  `json:"tid,omitempty"`
	IsHandshake bool    `json:"is_handshake"`
	LatencyMs   float64 `json:"latency_ms,omitempty"`
	DataType    string  `json:"data_type,omitempty"`
	DeltaNs     uint64  `json:"delta_ns,omitempty"`
}

type TLSLibraryStatus struct {
	Library   uint8  `json:"library"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Attached  bool   `json:"attached"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

type TLSCaptureStats struct {
	Pending        int                `json:"pending"`
	Dropped        int                `json:"dropped"`
	Timeout        time.Duration      `json:"timeout"`
	Libraries      []TLSLibraryStatus `json:"libraries,omitempty"`
	LastFragmentNS uint64             `json:"lastFragmentNs,omitempty"`
}

// bpfKtimeToWallClock converts a bpf_ktime_get_ns() timestamp (monotonic
// nanoseconds since system boot) to wall clock time. eBPF TLS uprobes use
// bpf_ktime_get_ns() which returns monotonic time since boot, not Unix epoch
// time. A direct time.Unix(0, ns) conversion would produce dates near 1970.
// This function detects that case and falls back to the current wall clock.
func bpfKtimeToWallClock(monoNS uint64) time.Time {
	t := time.Unix(0, int64(monoNS))
	// bpf_ktime_get_ns() is monotonic — its absolute value corresponds to a
	// date near boot time (1970 + uptime). Real wall-clock timestamps from
	// other sources will be >= 2020 for production data.
	if t.Year() < 2020 {
		return time.Now()
	}
	return t.UTC()
}
