package agentscope

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultAndIndependentLists(t *testing.T) {
	p := Default()
	if !Allows(p.Capture, "codex", "") || !Allows(p.Monitor, "codex", "") {
		t.Fatal("defaults must allow capture and monitor")
	}
	b, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(b), `"entries":[]`) {
		t.Fatalf("default JSON = %s (%v)", b, err)
	}
	p.Monitor = List{Mode: ModeWhitelist, Entries: []string{"codex"}}
	if !Allows(p.Capture, "other", "") || Allows(p.Monitor, "other", "") {
		t.Fatal("capture/monitor scope lists must be independent")
	}
}

func TestValidateList(t *testing.T) {
	got, err := ValidateList(List{Mode: ModeBlacklist, Entries: []string{" CoDeX ", "codex", "Gemini"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Entries, []string{"codex", "gemini"}) {
		t.Fatalf("entries = %#v", got.Entries)
	}
	for _, tc := range []List{
		{Mode: "allow", Entries: []string{}},
		{Mode: ModeWhitelist, Entries: []string{""}},
		{Mode: ModeWhitelist, Entries: []string{"line\nbreak"}},
		{Mode: ModeWhitelist, Entries: []string{string([]byte{0xff})}},
		{Mode: ModeBlacklist, Entries: []string{strings.Repeat("x", 129)}},
		{Mode: ModeBlacklist, Entries: make([]string, MaxEntries+1)},
	} {
		if _, err := ValidateList(tc); err == nil {
			t.Fatalf("accepted invalid list %#v", tc)
		}
	}
}

func TestAllowsWithOwner(t *testing.T) {
	list := List{Mode: ModeWhitelist, Entries: []string{"codex"}}
	if !AllowsWithOwner(list, "fish", "agent cli", "CODEX") {
		t.Fatal("verified owner should satisfy whitelist")
	}
	if AllowsWithOwner(list, "fish", "agent cli", "") {
		t.Fatal("unknown owner must not satisfy whitelist")
	}
	list.Mode = ModeBlacklist
	if AllowsWithOwner(list, "python", "", "codex") {
		t.Fatal("verified owner should also match blacklist")
	}
	if !AllowsWithOwner(list, "python", "", "") {
		t.Fatal("unattributed interpreter must not be denied")
	}
}

func TestValidateReportsCaptureAndMonitor(t *testing.T) {
	p := Default()
	p.Monitor.Mode = "unknown"
	if _, err := Validate(p); err == nil || !strings.Contains(err.Error(), "monitor:") {
		t.Fatalf("missing monitor context: %v", err)
	}
}
