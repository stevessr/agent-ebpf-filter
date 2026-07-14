package handlers

import (
	"testing"
	"unicode/utf8"
)

func TestBoundedHandlerText(t *testing.T) {
	value := "ab界cd\xff"
	got := boundedHandlerText(value, 5)
	if got != "ab界" || len(got) > 5 || !utf8.ValidString(got) {
		t.Fatalf("boundedHandlerText() = %q (%d bytes)", got, len(got))
	}
	if got := boundedHandlerText("value", 0); got != "" {
		t.Fatalf("zero limit result = %q", got)
	}
}
