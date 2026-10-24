package behaviorfeatures

import (
	"errors"
	"math"
	"mime"
	"net"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var errInvalidInput = errors.New("invalid or oversized behavior record")

var sqlHint = regexp.MustCompile(`(?i)(union\s+(all\s+)?select|\bselect\b.+\bfrom\b|\bor\s+['"0-9]+\s*=|sleep\s*\(|benchmark\s*\(|information_schema|--|/\*)`)
var xssHint = regexp.MustCompile(`(?i)(<\s*(script|svg|iframe|img)\b|javascript\s*:|\bon[a-z]+\s*=)`)
var cmdHint = regexp.MustCompile("(?i)(;|&&|\\|\\||`|\\$\\(|/bin/(sh|bash)|\\b(cmd|powershell)\\b)")
var urlHint = regexp.MustCompile(`(?i)(https?|file|gopher|dict)://`)

var payloadKeys = strings.Fields("bytes entropy digit_ratio alpha_ratio space_ratio nonascii_ratio control_ratio quote_count semicolon_count pipe_count percent_count slash_count backslash_count equals_count nul_count decode_rounds decode_error residual_encoding traversal_count sql_hint xss_hint command_hint nested_url_hint")
var pathKeys = strings.Fields("present truncated absolute relative unresolved lexical_resolved depth traversal_count sensitive_hint temporary_hint executable_hint")
var syscallKeys = strings.Fields("op_known op_read op_write op_exec op_open op_delete op_rename op_permission success failed errno actual_bytes requested_bytes open_read_intent open_write_intent open_create open_truncate open_append open_path_only open_tmpfile mode_exec mode_setid")
var httpKeys = strings.Fields("method_known method_get method_post method_put method_patch method_delete method_other outgoing incoming truncated target_present target_valid absolute_form query_count query_duplicate_keys encoded_delimiter path_depth path_traversal host_present authority_mismatch destination_ip_present ip_valid ip_private ip_loopback ip_linklocal ip_multicast ip_unspecified headers_present header_count duplicate_content_length transfer_encoding_with_content_length body_present body_bytes")

// Catalog is a stable, ordered feature contract independent of the legacy 128 dimensions.
var catalog = buildCatalog()

func Catalog() []string { return append([]string(nil), catalog...) }

func buildCatalog() []string {
	keys := []string{"capture_loss", "join_context_complete"}
	for _, p := range []string{"path.", "target_path."} {
		for _, k := range pathKeys {
			keys = append(keys, p+k)
		}
	}
	for _, k := range syscallKeys {
		keys = append(keys, "sys."+k)
	}
	for _, k := range httpKeys {
		keys = append(keys, "http."+k)
	}
	for _, p := range []string{"payload.", "body."} {
		for _, k := range payloadKeys {
			keys = append(keys, p+k)
		}
	}
	for _, w := range []string{"1s", "30s", "300s"} {
		for _, k := range []string{"prior_events", "prior_sensitive_reads", "prior_writes", "prior_http_outgoing"} {
			keys = append(keys, "history."+w+"."+k)
		}
	}
	return keys
}

func bound(s *string) bool { return s == nil || len(*s) <= MaxText }
func complete(c Context) bool {
	return c.HostID != "" && c.BootID != "" && c.RunID != "" && c.PIDNS != "" && c.TGID > 0 && c.StartTimeNS > 0 && c.Clock != "" && c.EventTimeNS > 0
}
func validInput(i Input) bool {
	n := 0
	if i.Syscall != nil {
		n++
		s := i.Syscall
		if len(s.Name) > 64 {
			return false
		}
		if !bound(s.Path) || !bound(s.TargetPath) || !bound(s.CWD) || !bound(s.DirFDPath) || !bound(s.FDPath) {
			return false
		}
	}
	if i.HTTP != nil {
		n++
		h := i.HTTP
		if !bound(h.Target) || !bound(h.Host) || !bound(h.DestinationIP) || !bound(h.Body) || len(h.Headers) > 256 || len(h.Method) > 32 || len(h.Direction) > 32 {
			return false
		}
		total := 0
		for _, v := range h.Headers {
			total += len(v.Name) + len(v.Value)
		}
		if total > MaxText {
			return false
		}
	}
	if i.Parameter != nil {
		n++
		if !bound(i.Parameter) {
			return false
		}
	}
	return n == 1 && len(i.Source) <= 256 && len(i.ID) <= 256 && len(i.Context.HostID) <= 256 && len(i.Context.BootID) <= 256 && len(i.Context.RunID) <= 256 && len(i.Context.PIDNS) <= 256 && len(i.Context.Clock) <= 64
}

// Extract never retains input text. An error deliberately contains no input content.
func Extract(i Input) (Output, error) {
	o := Output{Schema: SchemaVersion, Features: make(map[string]Value, len(catalog))}
	if !validInput(i) {
		return o, errInvalidInput
	}
	for _, k := range catalog {
		o.missing(k)
	}
	o.flag("join_context_complete", complete(i.Context))
	if i.CaptureLoss != nil {
		o.flag("capture_loss", *i.CaptureLoss)
	}
	switch {
	case i.Syscall != nil:
		o.Kind = "syscall"
		extractSys(&o, *i.Syscall)
	case i.HTTP != nil:
		o.Kind = "http_request"
		extractHTTP(&o, *i.HTTP)
	default:
		o.Kind = "http_parameter"
		payload(&o, *i.Parameter, true)
	}
	return o, nil
}

// Decode at most two layers. Query/form '+' handling is explicit and must not be
// applied to Linux file paths or URI path components.
func decode(s string, form bool) (string, int, bool) {
	rounds := 0
	for ; rounds < 2; rounds++ {
		var d string
		var e error
		if form {
			d, e = url.QueryUnescape(s)
		} else {
			d, e = url.PathUnescape(s)
		}
		if e != nil {
			return s, rounds, true
		}
		if d == s {
			break
		}
		s = d
	}
	return s, rounds, false
}
func traversals(s string) int {
	n := 0
	for _, p := range strings.Split(s, "/") {
		if p == ".." {
			n++
		}
	}
	return n
}
func payload(o *Output, raw string, form bool) {
	s, rounds, bad := decode(raw, form)
	o.put("payload.bytes", float64(len(raw)))
	o.put("payload.decode_rounds", float64(rounds))
	o.flag("payload.decode_error", bad)
	o.flag("payload.residual_encoding", regexpEncoding.MatchString(s))
	counts := [256]int{}
	digits, alpha, space, nonascii, control := 0, 0, 0, 0, 0
	for _, c := range []byte(s) {
		counts[c]++
		if c >= '0' && c <= '9' {
			digits++
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			alpha++
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			space++
		}
		if c >= 128 {
			nonascii++
		}
		if c < 32 || c == 127 {
			control++
		}
	}
	denom := math.Max(1, float64(len(s)))
	entropy := 0.
	for _, n := range counts {
		if n > 0 {
			p := float64(n) / denom
			entropy -= p * math.Log2(p)
		}
	}
	o.put("payload.entropy", entropy)
	for k, v := range map[string]int{"digit_ratio": digits, "alpha_ratio": alpha, "space_ratio": space, "nonascii_ratio": nonascii, "control_ratio": control} {
		o.put("payload."+k, float64(v)/denom)
	}
	for k, chars := range map[string]string{"quote_count": "'\"", "semicolon_count": ";", "pipe_count": "|", "percent_count": "%", "slash_count": "/", "backslash_count": "\\", "equals_count": "=", "nul_count": "\x00"} {
		n := 0
		for _, c := range []byte(chars) {
			n += counts[c]
		}
		o.put("payload."+k, float64(n))
	}
	o.put("payload.traversal_count", float64(traversals(s)))
	o.flag("payload.sql_hint", sqlHint.MatchString(s))
	o.flag("payload.xss_hint", xssHint.MatchString(s))
	o.flag("payload.command_hint", cmdHint.MatchString(s))
	o.flag("payload.nested_url_hint", urlHint.MatchString(s))
}

var regexpEncoding = regexp.MustCompile(`%[0-9a-fA-F]{2}`)

func extractPath(o *Output, prefix string, raw *string, base *string, truncated *bool) {
	o.flag(prefix+"present", raw != nil && *raw != "")
	if truncated != nil {
		o.flag(prefix+"truncated", *truncated)
	}
	if raw == nil || *raw == "" || truncated == nil || *truncated || strings.ContainsRune(*raw, 0) {
		return
	}
	s := *raw
	o.flag(prefix+"absolute", path.IsAbs(s))
	o.flag(prefix+"relative", !path.IsAbs(s))
	o.put(prefix+"traversal_count", float64(traversals(s)))
	resolved := path.IsAbs(s)
	if !resolved && base != nil && path.IsAbs(*base) && !strings.ContainsRune(*base, 0) {
		s = path.Join(*base, s)
		resolved = true
	}
	o.flag(prefix+"lexical_resolved", resolved)
	o.flag(prefix+"unresolved", !resolved)
	o.put(prefix+"depth", float64(len(strings.FieldsFunc(s, func(c rune) bool { return c == '/' }))))
	if !resolved {
		return
	}
	// Lexical categories only: symlink/mount/inode resolution is deliberately absent.
	s = path.Clean(s)
	sensitive := s == "/etc/shadow" || s == "/etc/gshadow" || s == "/etc/passwd" || strings.HasSuffix(s, "/.env") || strings.Contains(s, "/.ssh/") || strings.Contains(s, "/.aws/") || strings.Contains(s, "/.gnupg/")
	o.flag(prefix+"sensitive_hint", sensitive)
	o.flag(prefix+"temporary_hint", strings.HasPrefix(s, "/tmp/") || strings.HasPrefix(s, "/var/tmp/") || strings.HasPrefix(s, "/dev/shm/"))
	o.flag(prefix+"executable_hint", strings.HasPrefix(s, "/bin/") || strings.HasPrefix(s, "/usr/bin/") || strings.HasPrefix(s, "/usr/local/bin/"))
}
func extractSys(o *Output, s Syscall) {
	op := ""
	switch s.Name {
	case "read", "pread64", "preadv", "preadv2", "readv":
		op = "read"
	case "write", "pwrite64", "pwritev", "pwritev2", "writev":
		op = "write"
	case "execve", "execveat", "exec":
		op = "exec"
	case "open", "openat", "openat2", "creat":
		op = "open"
	case "unlink", "unlinkat", "rmdir":
		op = "delete"
	case "rename", "renameat", "renameat2":
		op = "rename"
	case "chmod", "fchmod", "fchmodat", "chown", "fchown", "lchown", "fchownat":
		op = "permission"
	}
	for _, k := range []string{"read", "write", "exec", "open", "delete", "rename", "permission"} {
		o.flag("sys.op_"+k, op == k)
	}
	o.flag("sys.op_known", op != "")
	if s.Return != nil {
		r := *s.Return
		o.flag("sys.success", r >= 0)
		o.flag("sys.failed", r < 0)
		errno := int64(0)
		if r < 0 && r >= -4095 {
			errno = -r
		}
		o.put("sys.errno", float64(errno))
		if op == "read" || op == "write" {
			n := r
			if n < 0 {
				n = 0
			}
			o.put("sys.actual_bytes", float64(n))
		}
	}
	if s.RequestedBytes != nil && (op == "read" || op == "write") {
		o.put("sys.requested_bytes", float64(*s.RequestedBytes))
	}
	if op == "open" && s.OpenFlags != nil {
		f := *s.OpenFlags
		access := f & 3
		pathOnly := f&0x200000 != 0
		o.flag("sys.open_read_intent", !pathOnly && (access == 0 || access == 2))
		o.flag("sys.open_write_intent", !pathOnly && (access == 1 || access == 2))
		o.flag("sys.open_path_only", pathOnly)
		o.flag("sys.open_tmpfile", f&0x410000 == 0x410000)
		o.flag("sys.open_create", f&64 != 0)
		o.flag("sys.open_truncate", f&512 != 0)
		o.flag("sys.open_append", f&1024 != 0)
	}
	if s.PermissionMode != nil {
		o.flag("sys.mode_exec", *s.PermissionMode&0111 != 0)
		o.flag("sys.mode_setid", *s.PermissionMode&06000 != 0)
	}
	p := s.Path
	base := s.CWD
	if op == "read" || op == "write" || s.Name == "fchmod" || s.Name == "fchown" {
		p = s.FDPath
		base = nil
	}
	switch s.Name {
	case "openat", "openat2", "execveat", "unlinkat", "renameat", "renameat2", "fchmodat", "fchownat":
		base = nil
		if s.DirFD != nil && *s.DirFD == -100 {
			base = s.CWD
		} else if s.DirFDPath != nil {
			base = s.DirFDPath
		}
	}
	if s.RelativeBaseUnknown {
		base = nil
	}
	extractPath(o, "path.", p, base, s.PathTruncated)
	// renameat has a separate target dirfd; do not reuse the source directory.
	targetBase := s.CWD
	if s.Name != "rename" || s.RelativeBaseUnknown {
		targetBase = nil
	}
	extractPath(o, "target_path.", s.TargetPath, targetBase, s.PathTruncated)
}

func extractHTTP(o *Output, h HTTP) {
	method := h.Method // HTTP method tokens are case-sensitive.
	known := method != ""
	o.flag("http.method_known", known)
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		if known {
			o.flag("http.method_"+strings.ToLower(m), method == m)
		}
	}
	if known {
		o.flag("http.method_other", method != "GET" && method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE")
	}
	if h.Direction == "outgoing" || h.Direction == "incoming" {
		o.flag("http.outgoing", h.Direction == "outgoing")
		o.flag("http.incoming", h.Direction == "incoming")
	}
	if h.Truncated != nil {
		o.flag("http.truncated", *h.Truncated)
	}
	o.flag("http.target_present", h.Target != nil)
	o.flag("http.host_present", h.Host != nil)
	o.flag("http.destination_ip_present", h.DestinationIP != nil)
	if h.DestinationIP != nil {
		ip := net.ParseIP(*h.DestinationIP)
		o.flag("http.ip_valid", ip != nil)
		if ip != nil {
			o.flag("http.ip_private", ip.IsPrivate())
			o.flag("http.ip_loopback", ip.IsLoopback())
			o.flag("http.ip_linklocal", ip.IsLinkLocalUnicast())
			o.flag("http.ip_multicast", ip.IsMulticast())
			o.flag("http.ip_unspecified", ip.IsUnspecified())
		}
	}
	if h.Target != nil {
		raw := *h.Target
		u, err := url.ParseRequestURI(raw)
		o.flag("http.target_valid", err == nil)
		if err == nil {
			o.flag("http.absolute_form", u.IsAbs())
			o.flag("http.encoded_delimiter", strings.Contains(strings.ToLower(raw), "%3f") || strings.Contains(strings.ToLower(raw), "%23"))
			// url.Path has already been decoded once: use EscapedPath for bounded decoding.
			dp, _, _ := decode(u.EscapedPath(), false)
			o.put("http.path_depth", float64(len(strings.FieldsFunc(dp, func(r rune) bool { return r == '/' }))))
			o.put("http.path_traversal", float64(traversals(dp)))
			count, duplicates := 0, 0
			seen := map[string]bool{}
			if u.RawQuery != "" {
				for _, pair := range strings.Split(u.RawQuery, "&") {
					if pair == "" {
						continue
					}
					count++
					k := strings.SplitN(pair, "=", 2)[0]
					k, _, _ = decode(k, true)
					if seen[k] {
						duplicates++
					}
					seen[k] = true
				}
			}
			o.put("http.query_count", float64(count))
			o.put("http.query_duplicate_keys", float64(duplicates))
			if u.Host != "" && h.Host != nil {
				o.flag("http.authority_mismatch", !strings.EqualFold(u.Host, *h.Host))
			}
		}
		// Preserve URL delimiter boundaries above; these lexical hints do not reparse them.
		payload(o, raw, false)
	}
	o.flag("http.headers_present", h.HeadersPresent)
	if h.HeadersPresent {
		o.put("http.header_count", float64(len(h.Headers)))
		cl, te := 0, 0
		for _, v := range h.Headers {
			switch strings.ToLower(v.Name) {
			case "content-length":
				cl++
			case "transfer-encoding":
				te++
			}
		}
		if h.HeadersPreserveDuplicates {
			o.flag("http.duplicate_content_length", cl > 1)
			o.flag("http.transfer_encoding_with_content_length", cl > 0 && te > 0)
		}
	}
	o.flag("http.body_present", h.Body != nil)
	if h.Body != nil {
		o.put("http.body_bytes", float64(len(*h.Body)))
		form := false
		for _, v := range h.Headers {
			if strings.EqualFold(v.Name, "content-type") {
				mediaType, _, err := mime.ParseMediaType(v.Value)
				if err == nil && mediaType == "application/x-www-form-urlencoded" {
					form = true
				}
			}
		}
		tmp := Output{Features: map[string]Value{}}
		payload(&tmp, *h.Body, form)
		for k, v := range tmp.Features {
			o.Features["body."+strings.TrimPrefix(k, "payload.")] = v
		}
	}
}
