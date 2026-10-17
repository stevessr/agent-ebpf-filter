package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func windowsTestProbe(supported []string, receipts map[string]string) harnessProbe {
	p := fakeHarnessProbe(supported, receipts, "C:/Users/Tester")
	p.goos = "windows"
	return p
}
func appxReceipt(name, family string) string {
	data, _ := json.Marshal(registeredWindowsApp{Name: name, PackageFamilyName: family, InstallLocation: "C:/Program Files/WindowsApps/test", Version: "1.2.3"})
	return string(data)
}
func appxReceiptKey(name string) string {
	return "powershell.exe -NoProfile -NonInteractive -Command " + appxProbeScript(name)
}

func TestWindowsPackageManagerCommandRouting(t *testing.T) {
	p := windowsTestProbe([]string{"winget", "choco", "scoop", "powershell.exe"}, nil)
	tests := []struct {
		source, pkg string
		want        []string
	}{
		{"winget", "Anthropic.Claude", []string{"/mock/bin/winget", "upgrade", "--id", "Anthropic.Claude", "--exact", "--source", "winget"}},
		{"msstore", "9NT1R1C2HH7J", []string{"/mock/bin/winget", "upgrade", "--id", "9NT1R1C2HH7J", "--exact", "--source", "msstore"}},
		{"scoop", "codex", []string{"/mock/bin/powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "scoop update codex"}},
		{"choco", "codex", []string{"/mock/bin/choco", "upgrade", "codex"}},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			got := harnessUpdateCommand(tc.source, tc.pkg, "", p)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	for _, bad := range []string{"evil;whoami", "--all", "../other", "foo'bar"} {
		if len(harnessUpdateCommand("scoop", bad, "", p)) != 0 {
			t.Fatal("accepted untrusted token", bad)
		}
	}
}

func TestWindowsScoopShimVerifiedByPrefixAndInventory(t *testing.T) {
	path := "C:\\Users\\Tester\\scoop\\shims\\codex.exe"
	receipts := map[string]string{
		"scoop prefix codex": "C:\\Users\\Tester\\scoop\\apps\\codex\\current",
		"scoop list":         "Installed apps:\n\nName Version Source\ncodex 0.1.0 main",
		"scoop which codex":  "C:\\Users\\Tester\\scoop\\apps\\codex\\current\\bin\\codex.exe",
	}
	p := windowsTestProbe([]string{"scoop", "powershell.exe"}, receipts)
	got := inspectHarnessInstallation("codex", "Codex", path, p)
	if got.Source != "scoop" || got.Package != "codex" || len(got.Command) == 0 {
		t.Fatalf("not detected: %+v", got)
	}
	receipts["scoop which codex"] = "C:\\Program Files\\Bad\\codex.exe"
	other := inspectHarnessInstallation("codex", "Codex", path, p)
	if other.Source == "scoop" || len(other.Command) > 0 {
		t.Fatalf("unverified Scoop shim accepted: %+v", other)
	}
}

func TestWindowsChocolateyOwnerMustBeExact(t *testing.T) {
	path := "C:/ProgramData/chocolatey/bin/codex.exe"
	receipts := map[string]string{"choco list --limit-output": "codex|1.2.3\nother|0.1"}
	p := windowsTestProbe([]string{"choco"}, receipts)
	got := inspectHarnessInstallation("codex", "Codex", path, p)
	if got.Source != "choco" || !reflect.DeepEqual(got.Command, []string{"/mock/bin/choco", "upgrade", "codex"}) {
		t.Fatalf("wrong choco source: %+v", got)
	}
	receipts["choco list --limit-output"] = "other|0.1"
	other := inspectHarnessInstallation("codex", "Codex", path, p)
	if other.Source == "choco" || len(other.Command) > 0 {
		t.Fatalf("unowned binary: %+v", other)
	}
}

func TestWindowsChatGPTStoreIdentityAndDifferentGenerations(t *testing.T) {
	older := "OpenAI.ChatGPT-Desktop_2p2nqsd0c76g0"
	newer := "OpenAI.Codex_2p2nqsd0c76g0"
	receipts := map[string]string{
		appxReceiptKey("OpenAI.ChatGPT-Desktop"):                 appxReceipt("OpenAI.ChatGPT-Desktop", older),
		"winget list --id 9NT1R1C2HH7J --exact --source msstore": "Name Id Version\nChatGPT 9NT1R1C2HH7J 1.0.0",
	}
	p := windowsTestProbe([]string{"powershell.exe", "winget"}, receipts)
	apps := windowsDesktopAppUpdates(p)
	if len(apps) != 2 || apps[1].Source != "msstore" || apps[1].Package != "9NT1R1C2HH7J" || len(apps[1].Command) == 0 {
		t.Fatalf("classic Store: %+v", apps)
	}
	receipts[appxReceiptKey("OpenAI.Codex")] = appxReceipt("OpenAI.Codex", newer)
	both := windowsDesktopAppUpdates(p)
	if both[1].Source != "multiple-msstore" || len(both[1].Command) > 0 {
		t.Fatalf("selected among ambiguous packages: %+v", both)
	}
	delete(receipts, appxReceiptKey("OpenAI.ChatGPT-Desktop"))
	receipts["winget list --id 9PLM9XGG6VKS --exact --source msstore"] = "ChatGPT 9PLM9XGG6VKS 2.3.4 msstore"
	current := windowsDesktopAppUpdates(p)
	if current[1].Source != "msstore" || current[1].Package != "9PLM9XGG6VKS" || len(current[1].Command) == 0 {
		t.Fatalf("current Store: %+v", current)
	}
}

func TestWindowsClaudeManagedMSIXNotForced(t *testing.T) {
	receipts := map[string]string{
		appxReceiptKey("Claude"): appxReceipt("Claude", "Claude_pzs8sxrjxfjjc"),
	}
	p := windowsTestProbe([]string{"powershell.exe", "winget"}, receipts)
	apps := windowsDesktopAppUpdates(p)
	if apps[0].Source != "msix" || len(apps[0].Command) > 0 {
		t.Fatalf("MSIX should defer to managed updater: %+v", apps[0])
	}
	receipts["winget list --id Anthropic.Claude --exact --source winget"] = "Claude Anthropic.Claude 1.2.3 winget"
	apps = windowsDesktopAppUpdates(p)
	if apps[0].Source != "winget-compatible" || apps[0].Package != "Anthropic.Claude" || len(apps[0].Command) == 0 {
		t.Fatalf("winget compatible: %+v", apps[0])
	}
	receipts[appxReceiptKey("Claude")] = appxReceipt("Claude", "Fake.Claude_bad")
	apps = windowsDesktopAppUpdates(p)
	if apps[0].Source != "missing" || len(apps[0].Command) > 0 {
		t.Fatalf("unverified MSIX identity accepted: %+v", apps[0])
	}
}

func TestWingetMatcherRejectsFalsePositiveText(t *testing.T) {
	p := windowsTestProbe([]string{"winget"}, map[string]string{
		"winget list --id Anthropic.Claude --exact --source winget": "No package found (Anthropic.Claude)",
	})
	if p.wingetInstalled("Anthropic.Claude", "winget") {
		t.Fatal("matched informational error text")
	}
	p.run = func(_ string, _ ...string) (string, error) { return "Other Other.App 1.2.3 winget", nil }
	if p.wingetInstalled("Anthropic.Claude", "winget") {
		t.Fatal("matched unrelated package")
	}
	p.run = func(_ string, _ ...string) (string, error) { return "Claude Anthropic.Claude 1.2.3 msstore", nil }
	if p.wingetInstalled("Anthropic.Claude", "winget") {
		t.Fatal("accepted wrong winget source")
	}
}

func TestWindowsProvenancePathsCaseInsensitiveAndNoEscape(t *testing.T) {
	if !winPathUnder("c:\\Users\\tester\\Scoop\\apps\\codex\\current\\a.exe", "C:/Users/Tester/scoop/apps") {
		t.Fatal("case-insensitive Windows ownership failed")
	}
	if winPathUnder("C:/Users/Tester/scoop/apps/../outside.exe", "C:/Users/Tester/scoop/apps") {
		t.Fatal("directory escape allowed")
	}
	if winPathUnder("D:/Users/Tester/scoop/apps/codex.exe", "C:/Users/Tester/scoop/apps") {
		t.Fatal("different drive accepted")
	}
	if !strings.Contains(strings.ToLower(winBase("C:\\ProgramData\\Chocolatey\\bin\\Codex.exe")), "codex") {
		t.Fatal("Windows executable basename failed")
	}
}

func TestWindowsScoopUsesPowerShellWhenNoExecutableShim(t *testing.T) {
	path := "C:/Users/Tester/scoop/shims/codex.exe"
	receipts := map[string]string{
		"powershell.exe -NoProfile -NonInteractive -Command scoop prefix codex": "C:/Users/Tester/scoop/apps/codex/current",
		"powershell.exe -NoProfile -NonInteractive -Command scoop list":         "codex 1.0 main",
		"powershell.exe -NoProfile -NonInteractive -Command scoop which codex":  "C:/Users/Tester/scoop/apps/codex/current/bin/codex.exe",
	}
	p := windowsTestProbe([]string{"powershell.exe"}, receipts)
	got := inspectHarnessInstallation("codex", "Codex", path, p)
	if got.Source != "scoop" || len(got.Command) == 0 {
		t.Fatalf("PowerShell-only Scoop not detected: %+v", got)
	}
	receipts["powershell.exe -NoProfile -NonInteractive -Command scoop which codex"] = "C:/Windows/not-owned.exe"
	other := inspectHarnessInstallation("codex", "Codex", path, p)
	if other.Source == "scoop" || len(other.Command) > 0 {
		t.Fatalf("unowned PowerShell Scoop shim: %+v", other)
	}
}

func TestWindowsNpmGlobalCmdShimWithManifestAndFixedShell(t *testing.T) {
	bin := "C:/Users/Tester/AppData/Roaming/npm/codex.cmd"
	prefix := "C:/Users/Tester/AppData/Roaming/npm"
	global := prefix + "/node_modules"
	receipts := map[string]string{
		"npm prefix -g": prefix,
		"npm root -g":   global,
	}
	p := windowsTestProbe([]string{"npm", "powershell.exe"}, receipts)
	original := p.lookup
	p.lookup = func(name string) (string, error) {
		if name == "npm" {
			return "C:/Program Files/nodejs/npm.cmd", nil
		}
		return original(name)
	}
	p.readFile = func(location string) ([]byte, error) {
		switch cleanWinPath(location) {
		case cleanWinPath(global + "/@openai/codex/package.json"):
			return []byte(`{"name":"@openai/codex"}`), nil
		case cleanWinPath(bin):
			return []byte("node_modules\\@openai\\codex\\bin\\codex.js"), nil
		}
		return nil, os.ErrNotExist
	}
	got := inspectHarnessInstallation("codex", "Codex", bin, p)
	if got.Source != "npm" || got.Package != "@openai/codex" {
		t.Fatalf("Windows npm shim not recognized: %+v", got)
	}
	expected := []string{"/mock/bin/powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "npm 'install' '-g' '@openai/codex@latest'"}
	if !reflect.DeepEqual(got.Command, expected) {
		t.Fatalf("Windows .cmd update should use quoted fixed PowerShell: %v", got.Command)
	}
	p.readFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	unknown := inspectHarnessInstallation("codex", "Codex", bin, p)
	if unknown.Source == "npm" || len(unknown.Command) > 0 {
		t.Fatalf("accepted shim without manifest: %+v", unknown)
	}
}
