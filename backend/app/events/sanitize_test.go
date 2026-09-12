package events

import (
	"bytes"
	"strings"
	"testing"
)

// legacySanitizeUTF8 is the previous string-based implementation, kept here as
// the reference semantics for the byte-view rewrite.
func legacySanitizeUTF8(b []byte) string {
	cleaned := strings.TrimRight(string(b), "\x00")
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")
	return strings.ToValidUTF8(cleaned, "�")
}

func TestSanitizeUTF8MatchesLegacySemantics(t *testing.T) {
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
			want := legacySanitizeUTF8(raw)
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

func TestTrimNULMatchesBytesTrimRight(t *testing.T) {
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
