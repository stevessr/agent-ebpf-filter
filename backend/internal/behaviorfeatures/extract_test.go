package behaviorfeatures

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }
func get(t *testing.T, i Input) Output {
	t.Helper()
	o, e := Extract(i)
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func want(t *testing.T, o Output, k string, v float64, p bool) {
	t.Helper()
	if o.Features[k] != (Value{v, p}) {
		t.Fatalf("%s: got %+v, want %v present=%v", k, o.Features[k], v, p)
	}
}
func TestSyscallSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		s    Syscall
		k    string
		v    float64
		p    bool
	}{
		{"requested_is_not_transferred", Syscall{Name: "read", RequestedBytes: ptr(uint64(4096)), Return: ptr(int64(7))}, "sys.actual_bytes", 7, true},
		{"failed_io", Syscall{Name: "write", Return: ptr(int64(-13))}, "sys.actual_bytes", 0, true},
		{"missing_return", Syscall{Name: "read"}, "sys.success", 0, false},
		{"open_fd_zero_success", Syscall{Name: "open", Return: ptr(int64(0))}, "sys.success", 1, true},
		{"mode_not_flags", Syscall{Name: "open", PermissionMode: ptr(uint64(0755))}, "sys.open_write_intent", 0, false},
		{"read_only_flags_zero", Syscall{Name: "open", OpenFlags: ptr(uint64(0))}, "sys.open_read_intent", 1, true},
		{"opath_is_not_read", Syscall{Name: "open", OpenFlags: ptr(uint64(0x200000))}, "sys.open_read_intent", 0, true},
		{"creat_uses_cwd", Syscall{Name: "creat", Path: ptr(".ssh/id_rsa"), CWD: ptr("/root")}, "path.sensitive_hint", 1, true},
		{"read_needs_fd_path", Syscall{Name: "read", Path: ptr("/etc/shadow")}, "path.present", 0, true},
		{"fd_path_sensitive", Syscall{Name: "read", FDPath: ptr("/etc/shadow")}, "path.sensitive_hint", 1, true},
		{"case_sensitive", Syscall{Name: "execve", Path: ptr("/ETC/shadow")}, "path.sensitive_hint", 0, true},
		{"at_missing_dirfd", Syscall{Name: "openat", Path: ptr(".ssh/id_rsa"), CWD: ptr("/root")}, "path.unresolved", 1, true},
		{"at_fdcwd", Syscall{Name: "openat", Path: ptr(".ssh/id_rsa"), CWD: ptr("/root"), DirFD: ptr(-100)}, "path.sensitive_hint", 1, true},
		{"at_known_base", Syscall{Name: "openat", Path: ptr("shadow"), DirFDPath: ptr("/etc")}, "path.sensitive_hint", 1, true},
		{"legacy_collapsed", Syscall{Name: "open", Path: ptr("shadow"), CWD: ptr("/etc"), RelativeBaseUnknown: true}, "path.unresolved", 1, true},
		{"path_truncated", Syscall{Name: "execve", Path: ptr("/etc/shadow"), PathTruncated: ptr(true)}, "path.sensitive_hint", 0, false},
		{"nul_path", Syscall{Name: "execve", Path: ptr("/etc/shadow\x00safe")}, "path.sensitive_hint", 0, false},
		{"backslash_not_separator", Syscall{Name: "execve", Path: ptr("/etc\\shadow")}, "path.sensitive_hint", 0, true},
		{"rename_target_unknown_base", Syscall{Name: "renameat", Path: ptr("a"), TargetPath: ptr(".ssh/id_rsa"), DirFDPath: ptr("/root"), CWD: ptr("/root")}, "target_path.sensitive_hint", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.s.PathTruncated == nil {
				tc.s.PathTruncated = ptr(false)
			}
			want(t, get(t, Input{Syscall: &tc.s}), tc.k, tc.v, tc.p)
		})
	}
}
func TestHTTPBoundaries(t *testing.T) {
	h := HTTP{Method: "GET", Target: ptr("/safe%3Fadmin=true?x=1&%78=2"), HeadersPresent: true, HeadersPreserveDuplicates: true, Headers: []Header{{"Content-Length", "1"}, {"content-length", "2"}, {"Transfer-Encoding", "chunked"}}}
	o := get(t, Input{HTTP: &h})
	want(t, o, "http.query_count", 2, true)
	want(t, o, "http.query_duplicate_keys", 1, true)
	want(t, o, "http.encoded_delimiter", 1, true)
	want(t, o, "http.duplicate_content_length", 1, true)
	want(t, o, "http.transfer_encoding_with_content_length", 1, true)
	want(t, o, "http.outgoing", 0, false)
	h.HeadersPreserveDuplicates = false
	o = get(t, Input{HTTP: &h})
	want(t, o, "http.duplicate_content_length", 0, false)
	h.Target = ptr("/%252e%252e/etc/shadow")
	o = get(t, Input{HTTP: &h})
	want(t, o, "http.path_traversal", 1, true)
	h.Target = ptr("http://example.test/a")
	h.Host = ptr("other.test")
	h.DestinationIP = ptr("::ffff:127.0.0.1")
	o = get(t, Input{HTTP: &h})
	want(t, o, "http.authority_mismatch", 1, true)
	want(t, o, "http.ip_loopback", 1, true)
	h.Body = ptr("%253Cscript%253E")
	o = get(t, Input{HTTP: &h})
	want(t, o, "body.xss_hint", 1, true)
}
func TestDecodeBoundAndMissing(t *testing.T) {
	for _, tc := range []struct {
		s, k string
		v    float64
	}{{"%25252e", "payload.decode_rounds", 2}, {"%25252e", "payload.residual_encoding", 1}, {"%xy", "payload.decode_error", 1}, {"a+b", "payload.space_ratio", 1.0 / 3}} {
		o := get(t, Input{Parameter: &tc.s})
		want(t, o, tc.k, tc.v, true)
		want(t, o, "http.method_known", 0, false)
		if o.Kind != "http_parameter" {
			t.Fatal(o.Kind)
		}
	}
}
func TestNoRawOutput(t *testing.T) {
	secret := "NEVER_EXPORT_credential_94201"
	o := get(t, Input{ID: secret, Source: secret, HTTP: &HTTP{Target: ptr("/" + secret + "?token=" + secret), Host: &secret, Body: &secret, Headers: []Header{{"Authorization", secret}}}})
	b, _ := json.Marshal(o)
	if strings.Contains(string(b), secret) {
		t.Fatal("raw content leaked")
	}
	keys := Catalog()
	if len(keys) != len(o.Features) {
		t.Fatal("catalog mismatch")
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Fatal("duplicate catalog name")
		}
		seen[k] = true
		if _, ok := o.Features[k]; !ok {
			t.Fatal(k)
		}
	}
	o2 := get(t, Input{HTTP: &HTTP{}})
	if !reflect.DeepEqual(o2, get(t, Input{HTTP: &HTTP{}})) {
		t.Fatal("nondeterministic")
	}
}
func TestBounds(t *testing.T) {
	want(t, get(t, Input{Syscall: &Syscall{Name: "open", Path: ptr("/etc/shadow")}}), "path.sensitive_hint", 0, false)
	for _, i := range []Input{{}, {Syscall: &Syscall{}, HTTP: &HTTP{}}, {Parameter: ptr(strings.Repeat("a", MaxText+1))}, {HTTP: &HTTP{Headers: make([]Header, 257)}}} {
		if _, e := Extract(i); e == nil {
			t.Fatal("invalid accepted")
		}
	}
	if _, e := ExtractBatch(make([]Input, MaxEvents+1)); e == nil {
		t.Fatal("oversized batch")
	}
}
func contextAt(t int64) Context {
	return Context{HostID: "host", BootID: "boot", RunID: "run", PIDNS: "namespace", TGID: 10, StartTimeNS: 20, Clock: "monotonic", EventTimeNS: t}
}
func TestHistoryIsolationAndCausality(t *testing.T) {
	read := Input{ID: "read", Source: "kernel", Context: contextAt(1e9), Syscall: &Syscall{Name: "read", FDPath: ptr("/etc/shadow"), PathTruncated: ptr(false), Return: ptr(int64(10))}}
	req := Input{ID: "http", Source: "tls", Context: contextAt(2e9), HTTP: &HTTP{Method: "POST", Direction: "outgoing"}}
	r, e := ExtractBatch([]Input{req, read, read})
	if e != nil {
		t.Fatal(e)
	}
	if r.Duplicates != 1 {
		t.Fatal(r)
	}
	want(t, r.Outputs[0], "history.1s.prior_sensitive_reads", 1, true)
	want(t, r.Outputs[1], "history.1s.prior_events", 0, true)
	for _, change := range []func(*Context){func(c *Context) { c.HostID = "other" }, func(c *Context) { c.BootID = "other" }, func(c *Context) { c.RunID = "other" }, func(c *Context) { c.PIDNS = "other" }, func(c *Context) { c.StartTimeNS++ }, func(c *Context) { c.Clock = "unix" }, func(c *Context) { c.EventTimeNS = 1e9 }} {
		q := req
		change(&q.Context)
		r, e = ExtractBatch([]Input{read, q})
		if e != nil {
			t.Fatal(e)
		}
		want(t, r.Outputs[1], "history.30s.prior_sensitive_reads", 0, true)
	}
	req.Context.StartTimeNS = 0
	r, e = ExtractBatch([]Input{read, req})
	if e != nil {
		t.Fatal(e)
	}
	want(t, r.Outputs[1], "history.30s.prior_events", 0, false)
	conflict := read
	conflict.Syscall = &Syscall{Name: "write"}
	if _, e = ExtractBatch([]Input{read, conflict}); e == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	other := read
	other.Source = "other"
	r, e = ExtractBatch([]Input{read, other})
	if e != nil || len(r.Outputs) != 2 {
		t.Fatal("cross-source dedupe")
	}
}
func BenchmarkExtractHTTP(b *testing.B) {
	i := Input{HTTP: &HTTP{Method: "POST", Target: ptr("/v1/data?x=%252e%252e&x=2"), Body: ptr("a=hello&b=world")}}
	for n := 0; n < b.N; n++ {
		if _, e := Extract(i); e != nil {
			b.Fatal(e)
		}
	}
}

func TestHTTPCaseAndMediaType(t *testing.T) {
	h := HTTP{Method: "get", Target: ptr("/"), HeadersPresent: true, Body: ptr("a+b"), Headers: []Header{{"Content-Type", "application/x-www-form-urlencoded-extra"}}}
	o := get(t, Input{HTTP: &h})
	want(t, o, "http.method_get", 0, true)
	want(t, o, "http.method_other", 1, true)
	want(t, o, "body.space_ratio", 0, true)
	h.Headers[0].Value = "application/x-www-form-urlencoded; charset=UTF-8"
	o = get(t, Input{HTTP: &h})
	want(t, o, "body.space_ratio", 1.0/3, true)
}
func TestCatalogCannotBeMutated(t *testing.T) {
	c := Catalog()
	c[0] = "injected"
	o := get(t, Input{Parameter: ptr("test")})
	if _, ok := o.Features["injected"]; ok {
		t.Fatal("shared catalog mutated")
	}
}
