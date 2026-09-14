from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing zero-alloc anchor in {path}: {old[:120]!r}")
    write(path, text.replace(old, new, 1))


profile = "backend/app/captureprofile/profile.go"
replace_once(profile,
'''func RequestPath(raw string) string {
\traw = strings.TrimSpace(raw)
\tif raw == "" {
\t\treturn ""
\t}
\tif parsed, err := url.Parse(raw); err == nil {
\t\tif parsed.Path != "" {
\t\t\treturn parsed.EscapedPath()
\t\t}
\t}
\tif index := strings.IndexAny(raw, "?#"); index >= 0 {
\t\traw = raw[:index]
\t}
\treturn raw
}''',
'''func RequestPath(raw string) string {
\traw = strings.TrimSpace(raw)
\tif raw == "" {
\t\treturn ""
\t}

\t// Capture parsers normally hand us a path that has already been separated
\t// from its authority. Avoid net/url.Parse on this overwhelmingly common hot
\t// path: it allocates even for simple strings such as "/v1/jobs/42". Only
\t// absolute URLs and authority-form references need URL parsing.
\tneedsURLParse := strings.HasPrefix(raw, "//") || strings.Contains(raw, "://")
\tif !needsURLParse {
\t\tif index := strings.IndexAny(raw, "?#"); index >= 0 {
\t\t\traw = raw[:index]
\t\t}
\t\treturn raw
\t}

\tif parsed, err := url.Parse(raw); err == nil {
\t\tif parsed.Path != "" {
\t\t\treturn parsed.EscapedPath()
\t\t}
\t}
\tif index := strings.IndexAny(raw, "?#"); index >= 0 {
\t\traw = raw[:index]
\t}
\treturn raw
}''')


test = "backend/app/captureprofile/profile_test.go"
text = read(test)
if "TestRequestPathFastPathAndAbsoluteURL" not in text:
    text += r'''

func TestRequestPathFastPathAndAbsoluteURL(t *testing.T) {
    cases := map[string]string{
        "/v1/jobs/42":                                  "/v1/jobs/42",
        "/v1/jobs/42?token=secret#fragment":            "/v1/jobs/42",
        "v1/jobs/42?token=secret":                       "v1/jobs/42",
        "https://api.example.test/v1/jobs/42?token=x":  "/v1/jobs/42",
        "//api.example.test/v1/jobs/42?token=x":         "/v1/jobs/42",
    }
    for input, want := range cases {
        if got := RequestPath(input); got != want {
            t.Fatalf("RequestPath(%q) = %q, want %q", input, got, want)
        }
    }
}

func TestRegistryMatchCompactZeroAllocsForPreparedPath(t *testing.T) {
    registry := NewRegistry([]Profile{{
        ID: "zero-alloc", Vendor: "acme", Product: "jobs", Operation: "create",
        Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"},
        Directions: []string{"outgoing"}, Methods: []string{"POST"}, Transports: []string{"tcp"},
        HostSuffixes: []string{"target.example.test"}, PathPrefixes: []string{"/v1/jobs"}, MinScore: 90,
    }})
    observation := Observation{
        Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST",
        Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42",
    }
    allocs := testing.AllocsPerRun(1000, func() {
        match, ok := registry.MatchCompact(observation)
        if !ok || match.ProfileID != "zero-alloc" {
            panic("compact target profile did not match")
        }
    })
    if allocs != 0 {
        t.Fatalf("MatchCompact hot path allocations = %.2f, want 0", allocs)
    }
}
'''
    write(test, text)
