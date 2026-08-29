package tls

import (
	"time"

	"golang.org/x/sys/unix"
)

// ---- moved from backend/zz_merged_backend.go section capturetypestls.go ----

// Compact perf samples make the on-wire size metadata+DataLen rather than the
// full Go/BPF struct, so increasing the scratch fragment does not penalize small
// TLS calls. 1984 keeps a full compact sample at 2044 bytes and doubles the
// capture window while leaving the verifier-visible loop bound unchanged.
const tlsFragmentSize = 1984
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
const tlsFuncRustlsEncryptOutgoing = 12   // rustls RecordLayer::encrypt_outgoing (SEND plaintext)
const tlsFuncRustlsConsumeFirstChunk = 13 // rustls Reader::consume + consume_first_chunk (RECV plaintext)

type tlsFragment struct {
	TimestampNS  uint64
	ConnectionID uint64
	PID          uint32
	TGID         uint32
	DataLen      uint32
	TotalLen     uint32
	OriginalLen  uint32
	FragIndex    uint16
	FragCount    uint16
	LibType      uint8
	Direction    uint8
	Flags        uint8
	Function     uint8
	Comm         [16]byte
	Data         [tlsFragmentSize]byte
}

type CompletedTLSFragment struct {
	TimestampNS  uint64
	ConnectionID uint64
	PID          uint32
	TGID         uint32
	DataLen      uint32
	TotalLen     uint32
	OriginalLen  uint32
	FragCount    uint16
	LibType      uint8
	Direction    uint8
	Flags        uint8
	Function     uint8
	Comm         string
	Payload      []byte
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

	// HTTP/2 metadata is safe protocol metadata. ConnectionID deliberately
	// remains internal because it is derived from a userspace pointer.
	HTTP2StreamID         uint32 `json:"http2_stream_id,omitempty"`
	HTTP2PromisedStreamID uint32 `json:"http2_promised_stream_id,omitempty"`
	HTTP2FrameType        string `json:"http2_frame_type,omitempty"`
	HTTP2Flags            uint8  `json:"http2_flags,omitempty"`

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

// bpfKtimeToWallClock converts bpf_ktime_get_ns() (CLOCK_MONOTONIC-like
// nanoseconds since boot) into a wall-clock timestamp while preserving when the
// probe actually fired. Returning time.Now() here would shift every event to
// userspace assembly time and destroy ordering/latency information.
func bpfKtimeToWallClock(monoNS uint64) time.Time {
	now := time.Now().UTC()
	if monoNS == 0 {
		return now
	}

	var currentMono unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &currentMono); err != nil {
		return now
	}
	if currentMono.Sec < 0 || currentMono.Nsec < 0 {
		return now
	}
	currentMonoNS := uint64(currentMono.Sec)*uint64(time.Second) + uint64(currentMono.Nsec)
	if monoNS > currentMonoNS {
		// A future monotonic timestamp cannot be mapped reliably; avoid emitting
		// a future wall-clock event because it would poison timeline ordering.
		return now
	}

	return now.Add(-time.Duration(currentMonoNS - monoNS))
}
