package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRelatedNavigationSetsExactFiltersAndReturnSource(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "会话"
	a.search = "stale search"
	a.eventPIDFilter = 999
	if !a.navigateSessionEvents("run:abc", true, true) {
		t.Fatal("session navigation refused a valid run key")
	}
	if a.page != "事件" || a.eventReturnPage != "会话" ||
		a.eventSessionFilter != "run:abc" || !a.eventFileEditsOnly ||
		!a.eventDelegatedOnly || a.search != "" || a.eventPIDFilter != 0 {
		t.Fatalf("session drilldown leaked or lost filters: %+v", a)
	}
	if !a.navigateTargetEvents("/workspace/main.py") {
		t.Fatal("valid exact target not navigated")
	}
	if a.eventTargetFilter != "/workspace/main.py" || a.eventSessionFilter != "" ||
		a.eventFileEditsOnly || a.eventDelegatedOnly || a.eventReturnPage != "会话" {
		t.Fatalf("target drilldown did not reset previous constraints: %+v", a)
	}
	a.clearEventFilters()
	if a.eventTargetFilter != "" || a.eventRootPIDFilter != 0 ||
		a.eventFileEditsOnly || a.eventDelegatedOnly {
		t.Fatal("clearing filters left the drilldown constraints active")
	}
}

func TestRelatedNavigationRejectsMissingEvidence(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "系统"
	for _, target := range []string{"", "-", "file write", "socket write",
		"目标路径或端点未记录", "无文件或网络操作对象", "foo\nbar"} {
		if a.navigateTargetEvents(target) {
			t.Fatalf("placeholder or unsafe target navigated: %q", target)
		}
	}
	if a.navigatePIDEvents(0) || a.navigateAgentEvents(-12) ||
		a.navigateSessionEvents("", false, false) || a.navigateRecognition(0, "   ") {
		t.Fatal("missing identifier navigated")
	}
	if a.page != "系统" || a.eventReturnPage != "" || a.eventTargetFilter != "" {
		t.Fatal("invalid relation changed navigation state")
	}
}

func TestRelatedNavigationRootAndDelegatedFileFilters(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "root", PID: 100, Comm: "codex", Tag: "Codex", Type: "write", Target: "/tmp/root", AgentRunID: "run-1"},
		{EventID: "bash-edit", PID: 101, RootAgentPID: 100, Comm: "bash", Type: "file_write", Target: "/tmp/a", AgentRunID: "run-1"},
		{EventID: "node-socket", PID: 102, RootAgentPID: 100, Comm: "node", Type: "write", Target: "socket write", AgentRunID: "run-1"},
		{EventID: "another-agent", PID: 100, RootAgentPID: 888, Comm: "python", Type: "file_write", Target: "/tmp/b", AgentRunID: "run-other"},
		{EventID: "plain", PID: 103, Comm: "fish", Type: "write", Target: "/tmp/c"},
	}
	if !a.navigateAgentEvents(100) {
		t.Fatal("root navigation rejected a real PID")
	}
	rows := a.filteredEvents()
	if len(rows) != 3 {
		t.Fatalf("root view should exclude other-root same-PID and plain shell: %+v", rows)
	}
	if !a.navigateSessionEvents("run:run-1", true, true) {
		t.Fatal("run navigation failed")
	}
	rows = a.filteredEvents()
	if len(rows) != 1 || rows[0].EventID != "bash-edit" {
		t.Fatalf("delegated file edit view mismatched: %+v", rows)
	}
	if !a.navigatePIDEvents(102) {
		t.Fatal("PID navigation failed")
	}
	rows = a.filteredEvents()
	if len(rows) != 1 || rows[0].EventID != "node-socket" {
		t.Fatalf("executor PID filter retained incompatible edit-only filter: %+v", rows)
	}
}

func TestExactTargetAndRootAreDistinctFromGlobalSearch(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{PID: 100, Type: "file_write", Target: "/tmp/file"},
		{PID: 101, Type: "file_write", Target: "/tmp/file.bak"},
	}
	if !a.navigateTargetEvents("/tmp/file") {
		t.Fatal("cannot navigate exact file target")
	}
	if rows := a.filteredEvents(); len(rows) != 1 || rows[0].Target != "/tmp/file" {
		t.Fatalf("exact target filter performed substring search: %+v", rows)
	}
}

func TestPathAccessJumpOnlyPrefillsValidatedAbsoluteFile(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "研判"
	a.pathAccessConfirm = true
	a.pathAccessConfirmText = "confirmation"
	for _, target := range []string{"", "relative.txt", "/tmp/../etc/passwd", "/tmp/a\nb", "file write", "https://example.com"} {
		if a.navigatePathAccess(target) {
			t.Fatalf("unsafe/nonexact file target navigated: %q", target)
		}
	}
	if a.page != "研判" {
		t.Fatal("invalid file target changed page")
	}
	if !a.navigatePathAccess("/workspace/src/main.go") {
		t.Fatal("valid exact file path should navigate to path permission draft")
	}
	if a.page != "路径权限" || a.pathAccessTarget != "/workspace/src/main.go" ||
		a.pathAccessConfirm || a.pathAccessConfirmText != "" {
		t.Fatalf("path draft did not reset confirmations: %+v", a)
	}
}

func TestDetailSummaryProvidesRelatedAgentIdentifiers(t *testing.T) {
	detail := map[string]any{
		"Event": map[string]any{
			"type": "file_write", "pid": float64(110), "ppid": float64(101),
			"rootAgentPid": float64(100), "comm": "python", "tag": "Agent CLI",
			"path": "/workspace/code.py", "agentRunId": "run-test",
		},
	}
	selected := eventDetailSelectedSummary(detail, "evt-1", nil)
	if selected.PID != 110 || selected.PPID != 101 || selected.RootAgentPID != 100 ||
		selected.Target != "/workspace/code.py" || selected.AgentRunID != "run-test" {
		t.Fatalf("detail navigation lost an associated parameter: %+v", selected)
	}
	if !isFileMutationSummary(selected) || !strings.HasPrefix(eventSessionKey(selected), "run:") {
		t.Fatal("detail should expose a navigable Agent file mutation")
	}
}

func TestRelatedQuickLinksAreClickable(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "研判"
	e := eventSummary{
		EventID: "edit", PID: 201, PPID: 200, RootAgentPID: 100,
		Comm: "python", Type: "file_write", Target: "/workspace/code.py",
		AgentRunID: "run-1",
	}
	a.events = []eventSummary{
		{EventID: "root", PID: 100, Comm: "codex", AgentRunID: "run-1"},
		e,
	}
	tester := ui.NewTester(func(c *ui.Context) {
		a.eventQuickLinks(c, e, true)
	}, 910, 240)
	tester.Frame()
	for _, label := range []string{"执行 PID", "父 PID", "归属 Agent", "同会话",
		"本会话文件修改", "委托编辑", "同目标", "Agent 识别", "路径权限"} {
		if !tester.HasText(label) {
			t.Errorf("missing link %q in incident context", label)
		}
	}
	if err := tester.Click("归属 Agent"); err != nil {
		t.Fatalf("root quick link not clickable: %v", err)
	}
	if a.page != "事件" || a.eventRootPIDFilter != 100 || a.eventReturnPage != "研判" {
		t.Fatalf("root quick link navigated incorrectly: %+v", a)
	}
}

func TestModalRelatedLinkClosesSensitiveDetail(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "事件"
	a.eventDetailOpen = true
	a.eventDetailID = "secret-1"
	a.eventDetail = map[string]any{"Event": map[string]any{"path": "/workspace/private.txt"}}
	e := eventSummary{
		EventID: "secret-1", PID: 201, RootAgentPID: 100, Comm: "bash",
		Type: "file_write", AgentRunID: "run-1", Target: "/workspace/private.txt",
	}
	tester := ui.NewTester(func(c *ui.Context) {
		a.eventDetailQuickLinks(c, e)
	}, 910, 260)
	tester.Frame()
	if err := tester.Click("同目标事件"); err != nil {
		t.Fatalf("same target link not clickable: %v", err)
	}
	if a.page != "事件" || a.eventTargetFilter != "/workspace/private.txt" ||
		a.eventDetailOpen || a.eventDetail != nil || a.eventDetailID != "" {
		t.Fatalf("modal link retained evidence payload or failed navigation: %+v", a)
	}
}

func TestAgentIdentificationLinkIsExactNotSubstring(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "研判"
	if !a.navigateRecognition(100, "codex") {
		t.Fatal("valid owner Agent was not located")
	}
	if a.page != "Agent 识别" || a.agentFocusPID != 100 ||
		a.agentReturnPage != "研判" || a.agentSearch != "100" {
		t.Fatalf("Agent jump failed to retain exact PID: %+v", a)
	}
	rows := []agentRecognitionRow{
		{Comm: "codex", PID: 100},
		{Comm: "codex", PID: 1001},
		{Comm: "claude", PID: 2100},
	}
	exact := filterAgentRecognitionRows(rows, a.agentSearch, a.agentFocusPID)
	if len(exact) != 1 || exact[0].PID != 100 {
		t.Fatalf("PID 100 navigation matched different processes: %+v", exact)
	}
	fuzzy := filterAgentRecognitionRows(rows, "codex", 0)
	if len(fuzzy) != 2 {
		t.Fatalf("manual fuzzy search should remain available: %+v", fuzzy)
	}
}

func TestPathPermissionJumpRemembersSourceWithoutApplyingPolicy(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "研判"
	if !a.navigatePathAccess("/home/test/project/settings.yaml") {
		t.Fatal("cannot prefill path")
	}
	if a.pathAccessReturnPage != "研判" || a.pathAccessConfirm {
		t.Fatalf("linked path should remain draft-only and offer return: %+v", a)
	}
}
