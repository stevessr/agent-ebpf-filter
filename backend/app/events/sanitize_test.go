package events

import (
	"bytes"
	"strings"
	"testing"
)

// kernelCStringReference defines the single-terminator semantics for fixed
// kernel buffers. Bytes after the first NUL must not be exposed to userspace.
func kernelCStringReference(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.ToValidUTF8(string(b), "�")
}

func TestSanitizeUTF8StopsAtFirstNUL(t *testing.T) {
	cases := map[string][]byte{
		"empty":                 nil,
		"all nul":               make([]byte, 16),
		"ascii padded":          append([]byte("claude-code"), make([]byte, 5)...),
		"exact fit":             []byte("0123456789abcdef"),
		"embedded nul":          append([]byte("arg0\x00arg1\x00arg2"), make([]byte, 8)...),
		"embedded nul only":     []byte("\x00\x00a\x00"),
		"invalid utf8":          append([]byte("caf\xe9\xff"), make([]byte, 4)...),
		"invalid utf8 with nul": []byte("a\x00\xffb\x00\x00"),
		"multibyte valid":       append([]byte("路径/文件.txt"), make([]byte, 3)...),
		"truncated rune":        []byte("abc\xe8\xb7"),
		"unicode replacement":   []byte("x�y\x00"),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			raw := append([]byte(nil), input...)
			want := kernelCStringReference(raw)
			got := SanitizeUTF8(raw)
			if got != want {
				t.Fatalf("SanitizeUTF8(%q) = %q, want %q", input, got, want)
			}
			// The result must never alias the caller's buffer, which is reused
			// for the next ring-buffer sample.
			for i := range raw {
				raw[i] = 'Z'
			}
			if got != want {
				t.Fatalf("SanitizeUTF8 result aliased the input buffer: %q", got)
			}
		})
	}
}

func TestTrimNULIsAView(t *testing.T) {
	buf := append([]byte("payload"), make([]byte, 9)...)
	view := TrimNUL(buf)
	if string(view) != "payload" || cap(view) != cap(buf) {
		t.Fatalf("TrimNUL = %q (cap %d), want view of original buffer", view, cap(view))
	}
	if got := TrimNUL(make([]byte, 4)); len(got) != 0 {
		t.Fatalf("TrimNUL(all NUL) len = %d, want 0", len(got))
	}
}

func TestTrimNULPreservesEmbeddedBinaryNUL(t *testing.T) {
	cases := [][]byte{
		nil,
		make([]byte, 256),
		append([]byte("claude"), make([]byte, 10)...),
		[]byte("no padding at all"),
		append([]byte("embedded\x00nul\x00then"), make([]byte, 5)...),
		[]byte("\x00\x00a"),
		[]byte("a\x00"),
		append(make([]byte, 0, 2048), append([]byte("huge tail"), make([]byte, 2000)...)...),
		{0},
	}
	for _, in := range cases {
		want := bytes.TrimRight(in, "\x00")
		got := TrimNUL(in)
		if !bytes.Equal(got, want) {
			t.Fatalf("TrimNUL(%q) = %q, want %q", in, got, want)
		}
		if len(got) > 0 && &got[0] != &in[0] {
			t.Fatal("TrimNUL returned a copy instead of a view")
		}
	}
}

func TestKernelCStringDoesNotSpliceReusedScratchPaths(t *testing.T) {
	// A short /proc path followed by stale bytes from a previous syscall
	// previously produced "/proc/self/statm/tmp/.../credentials.json".
	input := append([]byte("/proc/self/statm\x00"), []byte("/tmp/agent/credentials.json")...)
	if got := SanitizeUTF8(input); got != "/proc/self/statm" {
		t.Fatalf("path includes stale bytes beyond C-string terminator: %q", got)
	}
	if got := string(TrimCString(input)); got != "/proc/self/statm" {
		t.Fatalf("trimmed path includes stale bytes: %q", got)
	}
	// A NUL at offset zero must not disclose any trailing previous sample.
	if got := SanitizeUTF8([]byte("\x00secret-from-previous-event")); got != "" {
		t.Fatalf("empty kernel field leaked stale scratch data: %q", got)
	}
}

func TestTrimCStringDoesNotModifyBinaryPayloadView(t *testing.T) {
	binary := []byte{'D', 'N', 'S', 0, 'X', 0, 0}
	if got := TrimCString(binary); string(got) != "DNS" {
		t.Fatalf("C-string view = %q, want DNS", got)
	}
	if got := TrimNUL(binary); !bytes.Equal(got, []byte{'D', 'N', 'S', 0, 'X'}) {
		t.Fatalf("binary payload truncated embedded NUL: %q", got)
	}
}
