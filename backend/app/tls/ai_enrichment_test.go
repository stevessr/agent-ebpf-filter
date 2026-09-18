package tls

import "testing"

func TestDetectAIToolFromCommRecognizesZvecGrep(t *testing.T) {
	for _, comm := range []string{"zg", "zvec-grep"} {
		meta := detectAIToolFromComm(comm)
		if meta == nil {
			t.Fatalf("comm %q was not recognized", comm)
		}
		if meta.ToolName != "zvec-grep" || meta.ToolVendor != "zvec-ai" || meta.ToolType != "search_service" {
			t.Fatalf("comm %q metadata = %+v", comm, meta)
		}
	}
}

func TestDetectAIToolFromCmdlineRecognizesZvecGrepPackageAndNulArgs(t *testing.T) {
	for _, cmdline := range []string{
		"/usr/bin/node\x00/home/user/.npm/@zvec/zvec-grep/dist/cli/index.js\x00--server\x00--stdio\x00",
		"zg --server --stdio",
		"/home/user/.local/bin/zg --rg -n event",
	} {
		meta := detectAIToolFromCmdline(cmdline)
		if meta == nil || meta.ToolName != "zvec-grep" {
			t.Fatalf("cmdline %q metadata = %+v", cmdline, meta)
		}
	}
}

func TestDetectAIToolFromCmdlineDoesNotMatchUnrelatedZgText(t *testing.T) {
	if meta := detectAIToolFromCmdline("node /workspace/tool.js --message zg"); meta != nil {
		t.Fatalf("unrelated command was classified as zvec-grep: %+v", meta)
	}
}

func TestDetectAIToolRecognizesZCodeRuntime(t *testing.T) {
	for _, comm := range []string{"zcode", "ZCode"} {
		meta := detectAIToolFromComm(comm)
		if meta == nil {
			t.Fatalf("comm %q was not recognized", comm)
		}
		if meta.ToolName != "ZCode" || meta.ToolVendor != "Z.ai" || meta.ToolType != "ai_assistant" {
			t.Fatalf("comm %q metadata = %+v", comm, meta)
		}
		if meta.APIProvider != "" {
			t.Fatalf("ZCode runtime must not assume a model provider: %+v", meta)
		}
	}

	for _, cmdline := range []string{
		"zcode --verbose",
		"/usr/bin/zcode --project /workspace",
		"/home/user/Applications/ZCode.AppImage --no-sandbox",
	} {
		meta := detectAIToolFromCmdline(cmdline)
		if meta == nil || meta.ToolName != "ZCode" {
			t.Fatalf("cmdline %q metadata = %+v", cmdline, meta)
		}
	}
}
