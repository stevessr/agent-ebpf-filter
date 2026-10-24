// Package behaviorfeatures extracts bounded, metadata-only, offline behavior
// features. Source strings describe provenance; they do not authenticate it.
package behaviorfeatures

const SchemaVersion = "kernel-http-behavior.v2"
const MaxText = 65536
const MaxEvents = 20000
const MaxBatchBytes = 16 * 1024 * 1024

// Context must come from a collector's attribution, not an LLM guess. EventTimeNS
// is comparable only within the same Clock and BootID. StartTimeNS prevents PID reuse.
type Context struct {
	HostID      string `json:"host_id"`
	BootID      string `json:"boot_id"`
	RunID       string `json:"run_id"`
	PIDNS       string `json:"pid_namespace"`
	TGID        uint64 `json:"tgid"`
	StartTimeNS uint64 `json:"process_start_ns"`
	Clock       string `json:"clock"`
	EventTimeNS int64  `json:"event_time_ns"`
}
type Syscall struct {
	Name           string  `json:"name"`
	Path           *string `json:"path,omitempty"`
	TargetPath     *string `json:"target_path,omitempty"`
	CWD            *string `json:"cwd,omitempty"`
	DirFD          *int    `json:"dirfd,omitempty"`
	DirFDPath      *string `json:"dirfd_path,omitempty"`
	FDPath         *string `json:"fd_path,omitempty"`
	Return         *int64  `json:"return,omitempty"`
	RequestedBytes *uint64 `json:"requested_bytes,omitempty"`
	OpenFlags      *uint64 `json:"open_flags,omitempty"`
	PermissionMode *uint64 `json:"permission_mode,omitempty"`
	PathTruncated  *bool   `json:"path_truncated,omitempty"`
	// Legacy collapsed open/openat events cannot safely resolve relative paths.
	RelativeBaseUnknown bool `json:"relative_base_unknown"`
}
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type HTTP struct {
	Method                    string   `json:"method"`
	Target                    *string  `json:"target,omitempty"`
	Host                      *string  `json:"host,omitempty"`
	DestinationIP             *string  `json:"destination_ip,omitempty"`
	Direction                 string   `json:"direction"`
	Headers                   []Header `json:"headers,omitempty"`
	HeadersPresent            bool     `json:"headers_present"`
	HeadersPreserveDuplicates bool     `json:"headers_preserve_duplicates"`
	Body                      *string  `json:"body,omitempty"`
	Truncated                 *bool    `json:"truncated,omitempty"`
}

// Exactly one payload is required. Parameter samples are never HTTP/kernel events.
// ID is an opaque per-source collector identifier, not arbitrary raw user content.
type Input struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Context     Context  `json:"context"`
	Syscall     *Syscall `json:"syscall,omitempty"`
	HTTP        *HTTP    `json:"http,omitempty"`
	Parameter   *string  `json:"parameter,omitempty"`
	CaptureLoss *bool    `json:"capture_loss,omitempty"`
}
type Value struct {
	Value   float64 `json:"value"`
	Present bool    `json:"present"`
}

// Output deliberately excludes raw paths, parameters, bodies, IDs and scope keys.
type Output struct {
	InputIndex *int             `json:"input_index,omitempty"`
	Schema     string           `json:"schema"`
	Kind       string           `json:"kind"`
	Features   map[string]Value `json:"features"`
}

func (o *Output) put(k string, v float64) { o.Features[k] = Value{v, true} }
func (o *Output) flag(k string, v bool) {
	n := 0.
	if v {
		n = 1
	}
	o.put(k, n)
}
func (o *Output) missing(k string) { o.Features[k] = Value{} }

// Validate checks input bounds without allocating feature output.
func Validate(i Input) error {
	if !validInput(i) {
		return errInvalidInput
	}
	return nil
}
