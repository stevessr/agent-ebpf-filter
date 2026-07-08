//go:build recvlive

package tls

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Live test: attach rustls SEND+RECV uprobes to the real codex binary, run a
// `codex exec` prompt (which performs real TLS to chatgpt.com), and confirm we
// capture plaintext in both directions.
//
// Run: sudo go test -tags=recvlive -run TestRustlsRecvLive -v -timeout 120s ./app/tls/
func TestRustlsRecvLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	bin := "/home/steve/.local/share/pnpm/global/5/.pnpm/node_modules/@openai/codex-linux-x64/vendor/x86_64-unknown-linux-musl/bin/codex"

	store := NewTLSCaptureStore(4000)
	broadcaster := NewTLSCaptureBroadcaster()
	rules := NewTLSCaptureRuleStore()
	mgr, err := NewTLSProbeManager(store, broadcaster, rules)
	if err != nil {
		t.Skipf("probe manager unavailable (need root/CAP_BPF): %v", err)
	}
	defer mgr.Close()

	go mgr.ReadLoop()

	if err := mgr.AttachRustlsUprobes(bin, 0); err != nil {
		t.Fatalf("attach rustls uprobes: %v", err)
	}
	t.Log("rustls SEND+RECV uprobes attached to codex")

	cmd := exec.Command(bin, "exec", "reply with exactly: ping")
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	t.Logf("codex exec output (%d bytes):\n%s", len(out), string(out))
	if err != nil {
		t.Logf("codex exec returned error (may still have done TLS): %v", err)
	}

	time.Sleep(2 * time.Second)

	hits := mgr.ProbeHitCounters()
	t.Logf("probe hit counters: %+v", hits)

	events := store.Recent(0)
	t.Logf("captured events in store: %d", len(events))

	sendCnt, recvCnt := 0, 0
	for _, e := range events {
		switch e.Direction {
		case "send":
			sendCnt++
		case "recv":
			recvCnt++
		}
	}
	t.Logf("captured SEND events: %d, RECV events: %d", sendCnt, recvCnt)

	if sendCnt == 0 {
		t.Errorf("no SEND plaintext captured — encrypt_outgoing uprobe may not be firing")
	}
	if recvCnt == 0 {
		t.Errorf("no RECV plaintext captured — consume_first_chunk uprobe may not be firing")
	}

	// Show a sample of each direction for visual confirmation.
	shownSend, shownRecv := false, false
	for _, e := range events {
		if e.Direction == "send" && !shownSend {
			t.Logf("SEND sample: host=%s url=%s method=%s bodyLen=%d body=%q",
				e.Host, e.URL, e.Method, len(e.Body), truncate(e.Body, 120))
			shownSend = true
		}
		if e.Direction == "recv" && !shownRecv {
			t.Logf("RECV sample: host=%s url=%s status=%d bodyLen=%d body=%q",
				e.Host, e.URL, e.StatusCode, len(e.Body), truncate(e.Body, 120))
			shownRecv = true
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
