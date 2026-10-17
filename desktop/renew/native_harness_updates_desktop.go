package main

// Desktop application identity and Windows installer probes intentionally live
// beside the CLI updater, not in the privileged collector.
import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type registeredWindowsApp struct {
	Name              string `json:"Name"`
	PackageFamilyName string `json:"PackageFamilyName"`
	InstallLocation   string `json:"InstallLocation"`
	Version           string `json:"Version"`
}

// cleanWinPath is used only for comparing Windows paths. filepath.Rel would use
// the host OS separator in tests, so emulate Windows semantics in pure Go.
func cleanWinPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	return strings.ToLower(path.Clean(value))
}
func winPathUnder(value, base string) bool {
	value, base = cleanWinPath(value), cleanWinPath(base)
	return value != "" && base != "" && strings.HasPrefix(value, strings.TrimSuffix(base, "/")+"/")
}
func winSamePath(a, b string) bool {
	return cleanWinPath(a) != "" && cleanWinPath(a) == cleanWinPath(b)
}
func winBase(value string) string { return path.Base(cleanWinPath(value)) }
func (p harnessProbe) powershell() string {
	if p.has("powershell.exe") {
		return "powershell.exe"
	}
	if p.has("pwsh.exe") {
		return "pwsh.exe"
	}
	return ""
}

// Exactly three fixed identities are queried: CLI aliases are not proof of a
// desktop app, and Store names/locations are not matched fuzzily.
func appxProbeScript(name string) string {
	return "$p = Get-AppxPackage -Name '" + name + "' | Select-Object -First 1; if ($null -ne $p) { $p | Select-Object Name,PackageFamilyName,InstallLocation,@{Name='Version';Expression={$_.Version.ToString()}} | ConvertTo-Json -Compress }"
}
func (p harnessProbe) appx(name string) (registeredWindowsApp, bool) {
	if p.goos != "windows" || p.powershell() == "" {
		return registeredWindowsApp{}, false
	}
	allowed := map[string]string{
		"Claude":                 "Claude_pzs8sxrjxfjjc",
		"OpenAI.ChatGPT-Desktop": "OpenAI.ChatGPT-Desktop_2p2nqsd0c76g0",
		"OpenAI.Codex":           "OpenAI.Codex_2p2nqsd0c76g0",
	}
	family, ok := allowed[name]
	if !ok {
		return registeredWindowsApp{}, false
	}
	// Constant command text derived from the above whitelist, never user input.
	script := appxProbeScript(name)
	raw, err := p.run(p.powershell(), "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil || raw == "" {
		return registeredWindowsApp{}, false
	}
	var found registeredWindowsApp
	if json.Unmarshal([]byte(raw), &found) != nil || found.Name != name || found.PackageFamilyName != family || found.InstallLocation == "" {
		return registeredWindowsApp{}, false
	}
	return found, true
}

// WinGet 'list' discovers applicable manifests, not necessarily the tool that
// originally installed an EXE/MSIX. Only specific, exact installed package IDs
// can be used, and the UI must say winget-compatible instead of claiming owner.
func (p harnessProbe) wingetInstalled(id, source string) bool {
	if !p.has("winget") || cleanOwnedName(id) == "" || (source != "winget" && source != "msstore") {
		return false
	}
	raw, err := p.run("winget", "list", "--id", id, "--exact", "--source", source)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(raw, "\n") {
		columns := strings.Fields(line)
		if len(columns) < 3 {
			continue
		}
		// Filtered winget list can omit Source; if present, require agreement.
		if columns[len(columns)-1] == "winget" || columns[len(columns)-1] == "msstore" {
			if columns[len(columns)-1] != source {
				continue
			}
		}
		for i := 0; i < len(columns)-1; i++ {
			if columns[i] == id && columns[i+1] != "" {
				return true
			}
		}
	}
	return false
}

// Installed app matching is checked against the exact Scoop package prefix,
// a Scoop-generated shim and Scoop's recorded installed-package list.
func (p harnessProbe) runScoop(args ...string) (string, error) {
	if p.has("scoop") {
		return p.run("scoop", args...)
	}
	shell := p.powershell()
	if shell == "" {
		return "", os.ErrNotExist
	}
	for _, arg := range args {
		if cleanOwnedName(arg) == "" {
			return "", os.ErrInvalid
		}
	}
	return p.run(shell, "-NoProfile", "-NonInteractive", "-Command", "scoop "+strings.Join(args, " "))
}
func (p harnessProbe) scoopOwner(id, actual string) (string, bool) {
	if !p.has("scoop") && p.powershell() == "" {
		return "", false
	}
	root := os.Getenv("SCOOP")
	if root == "" {
		root = p.home + "/scoop"
	}
	if !winPathUnder(actual, root+"/apps") && !winPathUnder(actual, root+"/shims") {
		return "", false
	}
	name := id
	if id == "kiro" {
		name = "kiro-cli"
	}
	if cleanOwnedName(name) == "" {
		return "", false
	}
	prefix, err := p.runScoop("prefix", name)
	if err != nil || !winPathUnder(prefix, root+"/apps/"+name) {
		return "", false
	}
	list, err := p.runScoop("list")
	if err != nil {
		return "", false
	}
	found := false
	for _, row := range strings.Split(list, "\n") {
		words := strings.Fields(row)
		if len(words) >= 2 && words[0] == name {
			found = true
			break
		}
	}
	if !found {
		return "", false
	}
	if winPathUnder(actual, prefix) {
		return name, true
	}
	if !winPathUnder(actual, root+"/shims") {
		return "", false
	}
	if winBase(actual) != strings.ToLower(id)+".exe" && winBase(actual) != strings.ToLower(name)+".exe" {
		return "", false
	}
	owned, err := p.runScoop("which", id)
	if err != nil || !winPathUnder(strings.TrimSpace(owned), prefix) {
		return "", false
	}
	return name, true
}

// Chocolatey owns its bin shim directory. Require an installed package with
// the *exact* CLI name as well; other Chocolatey packages are left manual.
func (p harnessProbe) chocoOwner(id, actual string) (string, bool) {
	if !p.has("choco") {
		return "", false
	}
	root := os.Getenv("ChocolateyInstall")
	if root == "" {
		root = "C:/ProgramData/chocolatey"
	}
	if !winPathUnder(actual, root+"/bin") {
		return "", false
	}
	name := id
	if id == "kiro" {
		name = "kiro-cli"
	}
	if cleanOwnedName(name) == "" || winBase(actual) != strings.ToLower(name)+".exe" {
		return "", false
	}
	raw, err := p.run("choco", "list", "--limit-output")
	if err != nil {
		return "", false
	}
	for _, row := range strings.Split(raw, "\n") {
		pieces := strings.SplitN(strings.TrimSpace(row), "|", 2)
		if len(pieces) == 2 && strings.EqualFold(pieces[0], name) && pieces[1] != "" {
			return name, true
		}
	}
	return "", false
}

// Node's Windows global launchers are usually generated .cmd/.ps1 wrappers,
// not POSIX symlinks. Require manager global directories, manifest identity,
// and a wrapper that directly refers to that exact installed package.
func (p harnessProbe) windowsNodeShim(id, actual string) (source, pkg string) {
	if p.goos != "windows" {
		return "", ""
	}
	expected := harnessUpdatePackageNames[id]
	command := id
	if id == "deepseek" {
		command = "dsh"
	}
	if expected == "" {
		return "", ""
	}
	actualBase := winBase(actual)
	if actualBase != command+".cmd" && actualBase != command+".ps1" {
		return "", ""
	}
	for _, entry := range []struct{ source, binQuery, globalRootQuery, globalRootSource string }{
		{"npm", "prefix -g", "root -g", "npm"},
		{"yarn", "global bin", "global dir", "yarn"},
	} {
		if !p.has(entry.source) {
			continue
		}
		words := strings.Fields(entry.binQuery)
		binDir, err := p.run(entry.source, words...)
		if err != nil || !winSamePath(actual, strings.TrimSpace(binDir)+"/"+actualBase) {
			continue
		}
		rootArgs := strings.Fields(entry.globalRootQuery)
		globalRoot, err := p.run(entry.source, rootArgs...)
		if err != nil {
			continue
		}
		globalRoot = strings.TrimSpace(globalRoot)
		if entry.source == "yarn" {
			globalRoot += "/node_modules"
		}
		var metadata struct{ Name string }
		metadataBytes, err := p.readFile(globalRoot + "/" + expected + "/package.json")
		if err != nil || json.Unmarshal(metadataBytes, &metadata) != nil || metadata.Name != expected {
			continue
		}
		wrapper, err := p.readFile(actual)
		if err != nil || len(wrapper) > 32768 {
			continue
		}
		linkText := strings.ToLower(strings.ReplaceAll(string(wrapper), "\\", "/"))
		if !strings.Contains(linkText, "node_modules/"+strings.ToLower(expected)+"/") {
			continue
		}
		return entry.source, expected
	}
	return "", ""
}
func (p harnessProbe) windowsCLIOwner(id, actual string) (source, pkg, evidence string) {
	if p.goos != "windows" {
		return "", "", ""
	}
	if name, ok := p.scoopOwner(id, actual); ok {
		return "scoop", name, "Scoop 的应用前缀、已安装清单和启动入口一致"
	}
	if name, ok := p.chocoOwner(id, actual); ok {
		return "choco", name, "Chocolatey bin shim 与已安装包清单一致"
	}
	if src, name := p.windowsNodeShim(id, actual); src != "" {
		return src, name, "Windows 全局 shim、管理器目录和包清单均匹配"
	}
	return "", "", ""
}

func scanDesktopAppUpdates(p harnessProbe) []harnessUpdatePlan {
	switch p.goos {
	case "windows":
		return windowsDesktopAppUpdates(p)
	case "darwin":
		return []harnessUpdatePlan{
			macDesktopUpdate(p, "claude-desktop", "Claude Desktop", "Claude.app"),
			macDesktopUpdate(p, "chatgpt-desktop", "ChatGPT Desktop", "ChatGPT.app"),
		}
	default:
		path, _ := p.lookup("claude-desktop")
		claude := inspectHarnessInstallation("claude-desktop", "Claude Desktop", path, p)
		if claude.Source == "missing" {
			claude.Advisory = "Linux 官方 Claude Desktop 可使用 Anthropic APT 仓库更新；未发现 claude-desktop 命令。"
		}
		chatgpt := harnessUpdatePlan{
			ID: "chatgpt-desktop", Label: "ChatGPT Desktop", Source: "unknown",
			Advisory: "尚未找到可确认的 Linux 官方 ChatGPT Desktop 安装渠道；不会执行非官方更新脚本。",
		}
		return []harnessUpdatePlan{claude, chatgpt}
	}
}
func macDesktopUpdate(p harnessProbe, id, label, bundle string) harnessUpdatePlan {
	result := harnessUpdatePlan{ID: id, Label: label, Source: "missing", Advisory: "未发现官方桌面应用。"}
	roots := []string{"/Applications"}
	if p.home != "" {
		roots = append(roots, filepath.Join(p.home, "Applications"))
	}
	for _, root := range roots {
		app := filepath.Join(root, bundle)
		if info, err := os.Stat(app); err == nil && info.IsDir() {
			result.Source = "desktop-app"
			result.Path = app
			result.Evidence = "检测到 macOS Application Bundle"
			result.Advisory = "使用应用内检查更新，或原始分发渠道更新；不要用 CLI 包管理器覆盖桌面应用。"
			break
		}
	}
	return result
}
func windowsDesktopAppUpdates(p harnessProbe) []harnessUpdatePlan {
	claude := harnessUpdatePlan{ID: "claude-desktop", Label: "Claude Desktop", Source: "missing", Advisory: "未检测到已注册的 Claude Desktop MSIX。"}
	if app, ok := p.appx("Claude"); ok {
		claude.Path = "appx:" + app.PackageFamilyName
		claude.Package = app.Name
		claude.Source = "msix"
		claude.Evidence = "已注册的 Windows MSIX：" + app.PackageFamilyName + " · " + app.Version
		claude.Advisory = "使用 Claude 应用内更新、Microsoft Store 或企业 MDM。MSIX 可能由官方安装器管理，不可单凭 winget 匹配判定原始安装来源。"
		if p.wingetInstalled("Anthropic.Claude", "winget") {
			claude.Source = "winget-compatible"
			claude.Package = "Anthropic.Claude"
			claude.Command = harnessUpdateCommand("winget", "Anthropic.Claude", "", p)
			claude.Evidence += " · winget 安装清单匹配"
			claude.Advisory = "可尝试 winget 更新该已注册应用；这不证明原始安装渠道。如由 Claude 自动更新或企业 MDM 托管，请不要同时使用 winget。"
		}
	} else {
		// Legacy unverified Win32 installs remain read-only.
		local := os.Getenv("LOCALAPPDATA")
		if local != "" {
			binary := filepath.Join(local, "Programs", "Claude", "Claude.exe")
			if info, err := os.Stat(binary); err == nil && !info.IsDir() {
				claude.Path = binary
				claude.Source = "unverified-exe"
				claude.Evidence = "本地旧版安装目录存在可执行文件"
				claude.Advisory = "未验证安装器身份：请使用 Claude Desktop 官方更新功能或原安装器。"
			}
		}
	}
	chatgpt := harnessUpdatePlan{ID: "chatgpt-desktop", Label: "ChatGPT Desktop", Source: "missing", Advisory: "未检测到 Microsoft Store ChatGPT Desktop。"}
	// Both Store generations can coexist. Never assume which one is active.
	classic, foundClassic := p.appx("OpenAI.ChatGPT-Desktop")
	current, foundCurrent := p.appx("OpenAI.Codex")
	if foundClassic && foundCurrent {
		chatgpt.Source = "multiple-msstore"
		chatgpt.Evidence = "已检测到两个不同的 OpenAI 签名 Store 应用包"
		chatgpt.Advisory = "同时检测到新旧 ChatGPT Store 包；请在 Microsoft Store 中分别确认更新，不自动选择其中一个。"
	} else if foundClassic || foundCurrent {
		app, storeID := classic, "9NT1R1C2HH7J"
		if foundCurrent {
			app, storeID = current, "9PLM9XGG6VKS"
		}
		chatgpt.Path = "appx:" + app.PackageFamilyName
		chatgpt.Package = app.Name
		chatgpt.Source = "msstore"
		chatgpt.Evidence = "已注册的 Microsoft Store 包：" + app.PackageFamilyName + " · " + app.Version
		chatgpt.Advisory = "优先使用 Microsoft Store 获取更新；仅在 winget 确认同一 Store 产品 ID 时允许执行指定包更新。"
		if p.wingetInstalled(storeID, "msstore") {
			chatgpt.Package = storeID
			chatgpt.Command = harnessUpdateCommand("msstore", storeID, "", p)
			chatgpt.Evidence += " · Store 产品 ID 已匹配"
		}
	}
	return []harnessUpdatePlan{claude, chatgpt}
}
