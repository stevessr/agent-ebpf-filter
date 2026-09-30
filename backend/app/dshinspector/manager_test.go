package dshinspector

import "testing"

func TestValidateLoopbackWebSocket(t *testing.T) {
	for _, endpoint := range []string{
		"ws://127.0.0.1:9230/devtools/page/1",
		"ws://[::1]:9230/devtools/page/1",
		"ws://localhost:9230/devtools/page/1",
	} {
		if err := validateLoopbackWebSocket(endpoint); err != nil {
			t.Fatalf("%s: %v", endpoint, err)
		}
	}
	if err := validateLoopbackWebSocket("ws://example.com:9230/devtools/page/1"); err == nil {
		t.Fatal("expected non-loopback endpoint to be rejected")
	}
}

func TestParsePort(t *testing.T) {
	if port, err := ParsePort(""); err != nil || port != 0 {
		t.Fatalf("empty port = %d, %v", port, err)
	}
	if port, err := ParsePort("9230"); err != nil || port != 9230 {
		t.Fatalf("9230 = %d, %v", port, err)
	}
	if _, err := ParsePort("70000"); err == nil {
		t.Fatal("expected invalid port error")
	}
}
