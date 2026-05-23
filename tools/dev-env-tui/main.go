package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type envVar struct {
	Key    string
	Label  string
	Hint   string
	Secret bool
}

type envGroup struct {
	ID    string
	Title string
	Desc  string
	Vars  []envVar
}

var groups = []envGroup{
	{
		ID:    "core",
		Title: "Core",
		Desc:  "backend/dev basics, auth, hooks, shell",
		Vars: []envVar{
			{Key: "DISABLE_AUTH", Label: "Disable API auth", Hint: "true is recommended for local make dev; release installs still use runtime auth."},
			{Key: "GIN_MODE", Label: "Gin mode", Hint: "debug or release. Use debug while developing."},
			{Key: "AGENT_API_KEY", Label: "Runtime API token seed", Hint: "Seeds the backend runtime token when runtime.json has none; adapters also use it.", Secret: true},
			{Key: "AGENT_ACCESS_TOKEN", Label: "Smoke/API token alias", Hint: "Used by smoke scripts; backend token preference is AGENT_API_KEY.", Secret: true},
			{Key: "AGENT_BACKEND_PORT", Label: "Fixed backend port", Hint: "Leave blank for auto-select 8080..8089."},
			{Key: "AGENT_REAL_HOME", Label: "Real user home", Hint: "Use when sudo/pkexec would otherwise store config under /root."},
			{Key: "AGENT_WRAPPER_PATH", Label: "Wrapper path", Hint: "Leave blank to use ./agent-wrapper built by make dev."},
			{Key: "AGENT_HOOK_ENDPOINT", Label: "Hook endpoint", Hint: "Optional callback URL; blank means hook scripts read backend/.port."},
			{Key: "AGENT_SHELL_DIR", Label: "Shell directory", Hint: "Optional working directory for backend shell sessions."},
			{Key: "AGENT_EBPF_DEV_SESSION", Label: "Zellij session", Hint: "Used by make dev / scripts/dev-zellij.sh."},
		},
	},
	{
		ID:    "ml-llm",
		Title: "ML / LLM",
		Desc:  "ML parameters and OpenAI-compatible LLM scoring/compiler config",
		Vars: []envVar{
			{Key: "AGENT_ML_ENABLED", Label: "Enable ML", Hint: boolHint()},
			{Key: "AGENT_ML_MODEL_TYPE", Label: "Model type", Hint: "random_forest, logistic, svm, knn, naive_bayes, nearest_centroid, extra_trees, adaboost, ensemble, etc."},
			{Key: "AGENT_ML_MODEL_PATH", Label: "Model path", Hint: "Leave blank for ~/.config/agent-ebpf-filter/ml_model.bin."},
			{Key: "AGENT_ML_AUTO_TRAIN", Label: "Auto train", Hint: boolHint()},
			{Key: "AGENT_ML_TRAIN_INTERVAL", Label: "Train interval", Hint: "Duration such as 24h."},
			{Key: "AGENT_ML_MIN_SAMPLES_FOR_TRAINING", Label: "Min training samples", Hint: "Positive integer."},
			{Key: "AGENT_ML_BLOCK_CONFIDENCE_THRESHOLD", Label: "Block confidence", Hint: "Float threshold, e.g. 0.85."},
			{Key: "AGENT_ML_MIN_CONFIDENCE", Label: "Min confidence", Hint: "Float threshold, e.g. 0.60."},
			{Key: "AGENT_ML_LOW_ANOMALY_THRESHOLD", Label: "Low anomaly", Hint: "Float threshold, e.g. 0.30."},
			{Key: "AGENT_ML_HIGH_ANOMALY_THRESHOLD", Label: "High anomaly", Hint: "Float threshold, e.g. 0.70."},
			{Key: "AGENT_ML_ACTIVE_LEARNING_ENABLED", Label: "Active learning", Hint: boolHint()},
			{Key: "AGENT_ML_FEATURE_HISTORY_SIZE", Label: "Feature history", Hint: "Positive integer."},
			{Key: "AGENT_ML_NUM_TREES", Label: "Tree count", Hint: "Random forest / profile integer."},
			{Key: "AGENT_ML_MAX_DEPTH", Label: "Max depth", Hint: "Tree/profile integer."},
			{Key: "AGENT_ML_MIN_SAMPLES_LEAF", Label: "Min samples leaf", Hint: "Tree/profile integer."},
			{Key: "AGENT_ML_VALIDATION_SPLIT_RATIO", Label: "Validation split", Hint: "Float ratio, e.g. 0.20."},
			{Key: "AGENT_ML_BALANCE_CLASSES", Label: "Balance classes", Hint: boolHint()},
			{Key: "AGENT_LLM_ENABLED", Label: "Enable LLM", Hint: "true enables OpenAI-compatible LLM scoring and NLP Blocks Compiler backend startup override."},
			{Key: "AGENT_LLM_BASE_URL", Label: "LLM base URL", Hint: "OpenAI-compatible base URL, e.g. http://127.0.0.1:11434/v1 or https://api.openai.com/v1."},
			{Key: "AGENT_LLM_API_KEY", Label: "LLM API key", Hint: "Stored only in local .env.dev; redacted in previews.", Secret: true},
			{Key: "AGENT_LLM_MODEL", Label: "LLM model", Hint: "Example: qwen2.5-coder, gpt-4.1-mini, llama3.1."},
			{Key: "AGENT_LLM_TIMEOUT_SECONDS", Label: "LLM timeout", Hint: "Seconds; runtime default is 45."},
			{Key: "AGENT_LLM_TEMPERATURE", Label: "LLM temperature", Hint: "0..2; use 0 for deterministic policy/compiler output."},
			{Key: "AGENT_LLM_MAX_TOKENS", Label: "LLM max tokens", Hint: "Backend clamps request sizes; blank keeps Runtime Config."},
			{Key: "AGENT_LLM_SYSTEM_PROMPT", Label: "System prompt", Hint: "Optional LLM scoring system prompt override."},
			{Key: "OPENAI_BASE_URL", Label: "OpenAI base fallback", Hint: "Fallback when AGENT_LLM_BASE_URL is blank."},
			{Key: "OPENAI_API_KEY", Label: "OpenAI key fallback", Hint: "Fallback when AGENT_LLM_API_KEY is blank.", Secret: true},
			{Key: "OPENAI_MODEL", Label: "OpenAI model fallback", Hint: "Fallback when AGENT_LLM_MODEL is blank."},
		},
	},
	{
		ID:    "app",
		Title: "App Behavior",
		Desc:  "runtime toggles, OTLP/TLS/domain-forwarding, sandbox, cluster",
		Vars: []envVar{
			{Key: "AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED", Label: "Persist event log", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_LOG_FILE_PATH", Label: "Event log path", Hint: "Default: ~/.config/agent-ebpf-filter/events.jsonl."},
			{Key: "AGENT_RUNTIME_MAX_EVENT_COUNT", Label: "Retention count", Hint: "Recent event retention count."},
			{Key: "AGENT_RUNTIME_MAX_EVENT_AGE", Label: "Retention max age", Hint: "Duration like 0, 5m, 24h. 0 disables age eviction."},
			{Key: "AGENT_RUNTIME_SHELL_SESSIONS_ENABLED", Label: "Shell sessions", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_SYSTEM_RUN_ENABLED", Label: "System run", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED", Label: "Hook management", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED", Label: "Policy/plugin mutations", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_TLS_CAPTURE_ENABLED", Label: "TLS capture", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_OTLP_ENABLED", Label: "OTLP export", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_OTLP_ENDPOINT", Label: "OTLP endpoint", Hint: "Example: http://127.0.0.1:4318/v1/traces."},
			{Key: "AGENT_RUNTIME_OTLP_SERVICE_NAME", Label: "OTLP service", Hint: "Default runtime value is agent-ebpf-filter."},
			{Key: "AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED", Label: "Domain forwarder", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_DOMAIN_HTTP_PORT", Label: "Domain HTTP port", Hint: "Default runtime value is 80."},
			{Key: "AGENT_RUNTIME_DOMAIN_HTTPS_PORT", Label: "Domain HTTPS port", Hint: "Default runtime value is 443."},
			{Key: "AGENT_RUNTIME_DOMAIN_DEFAULT_SCHEME", Label: "Default upstream scheme", Hint: "http or https. Runtime default is https."},
			{Key: "AGENT_RUNTIME_DOMAIN_ALLOW_ANY_HOST", Label: "Allow any host", Hint: boolHint()},
			{Key: "AGENT_RUNTIME_DOMAIN_DNS_RESOLVER", Label: "DNS resolver", Hint: "Optional resolver host:port."},
			{Key: "AGENT_RUNTIME_DOMAIN_DIAL_TIMEOUT_SECONDS", Label: "Dial timeout", Hint: "Seconds; runtime clamps 1..120."},
			{Key: "AGENT_RUNTIME_DOMAIN_CERT_FILE", Label: "Default cert file", Hint: "Required for HTTPS listener unless route certs cover traffic."},
			{Key: "AGENT_RUNTIME_DOMAIN_KEY_FILE", Label: "Default key file", Hint: "Required with cert file for HTTPS listener."},
			{Key: "AGENT_CGROUP_SANDBOX_PATH", Label: "cgroup attach path", Hint: "Default: /sys/fs/cgroup."},
			{Key: "AGENT_EBPF_BOOTSTRAP", Label: "Force bootstrap", Hint: boolHint()},
			{Key: "AGENT_EBPF_NO_SANDBOX", Label: "Disable sandbox", Hint: boolHint()},
			{Key: "AGENT_EBPF_SANDBOX_STRICT", Label: "Strict sandbox", Hint: boolHint()},
			{Key: "AGENT_EBPF_NO_CAP_DROP", Label: "No cap drop", Hint: boolHint()},
			{Key: "AGENT_EBPF_NO_NO_NEW_PRIVS", Label: "No no_new_privs", Hint: boolHint()},
			{Key: "AGENT_CLUSTER_MASTER_URL", Label: "Cluster master URL", Hint: "When set, backend registers as a cluster node."},
			{Key: "AGENT_CLUSTER_NODE_URL", Label: "Cluster node URL", Hint: "Node callback URL exposed to the master."},
			{Key: "AGENT_CLUSTER_NODE_ID", Label: "Cluster node ID", Hint: "Optional stable node id."},
			{Key: "AGENT_CLUSTER_NODE_NAME", Label: "Cluster node name", Hint: "Optional display name."},
			{Key: "AGENT_CLUSTER_ACCOUNT", Label: "Cluster account", Hint: "Account for registering with the master."},
			{Key: "AGENT_CLUSTER_PASSWORD", Label: "Cluster password", Hint: "Stored only in local .env.dev; redacted in previews.", Secret: true},
		},
	},
	{
		ID:    "devcontainer",
		Title: "Devcontainer",
		Desc:  "GHCR image and local mounted container options",
		Vars: []envVar{
			{Key: "CONTAINER_CLI", Label: "Container CLI", Hint: "docker or podman; blank means auto-detect."},
			{Key: "DEV_BRANCH", Label: "Branch override", Hint: "Leave blank for Makefile auto-detect."},
			{Key: "DEV_IMAGE_REPOSITORY", Label: "GHCR repository", Hint: "Leave blank for Makefile auto-detect."},
			{Key: "DEV_IMAGE_TAG", Label: "Image tag", Hint: "Leave blank for Makefile branch hash tag."},
			{Key: "DEV_IMAGE", Label: "Full image", Hint: "Usually blank so repository/tag are derived."},
			{Key: "DEV_CONTAINER", Label: "Container name", Hint: "Used by make exec."},
			{Key: "DEV_WORKSPACE", Label: "Workspace path", Hint: "Keep aligned with .devcontainer/devcontainer.json."},
			{Key: "DEVCONTAINER_POSTCREATE_INSTALL", Label: "Online post-create", Hint: "0 keeps offline fail-fast behavior; 1 explicitly allows make predev online."},
		},
	},
	{
		ID:    "tooling",
		Title: "Tooling",
		Desc:  "CUDA, smoke tests, runtime replay, ML sweep",
		Vars: []envVar{
			{Key: "CUDA_GO_TAGS", Label: "CUDA Go tags", Hint: "Use cuda only when /opt/cuda has nvcc and runtime libs; blank unsets."},
			{Key: "OS_SMOKE_PRIVILEGE_CMD", Label: "Smoke privilege cmd", Hint: "Example: sudo -E."},
			{Key: "OS_SMOKE_BACKEND_CMD", Label: "Smoke backend cmd", Hint: "Override backend binary used by smoke script."},
			{Key: "OS_SMOKE_BACKEND_LOG", Label: "Smoke backend log", Hint: "Default: /tmp/agent-ebpf-os-smoke-backend.log."},
			{Key: "RUNTIME_REPLAY_OUT", Label: "Replay output JSON", Hint: "Runtime replay summary JSON path."},
			{Key: "RUNTIME_REPLAY_OUTDIR", Label: "Replay output dir", Hint: "Runtime replay output directory."},
			{Key: "ML_SWEEP", Label: "ML sweep mode flag", Hint: "Set to 1 to run sweep test path."},
			{Key: "ML_SWEEP_MODE", Label: "Sweep mode", Hint: "quick, full, or comprehensive."},
			{Key: "ML_SWEEP_MODELS", Label: "Sweep models", Hint: "Comma-separated model filter."},
			{Key: "ML_SWEEP_DATASETS", Label: "Sweep datasets", Hint: "Comma-separated dataset filter."},
			{Key: "ML_SWEEP_WORKERS", Label: "Sweep workers", Hint: "Positive integer."},
			{Key: "ML_SWEEP_POINTS_PER_PARAM", Label: "Sweep points", Hint: "Positive integer."},
			{Key: "ML_SWEEP_REPEATS", Label: "Sweep repeats", Hint: "Positive integer."},
			{Key: "ML_SWEEP_STABILITY_TOP", Label: "Stability top-N", Hint: "Positive integer."},
			{Key: "ML_SWEEP_OUTDIR", Label: "Sweep output dir", Hint: "Directory for sweep reports."},
			{Key: "ML_SWEEP_RESUME", Label: "Resume sweep", Hint: boolHint()},
			{Key: "ML_SWEEP_QUIET_LOGS", Label: "Quiet sweep logs", Hint: boolHint()},
		},
	},
}

func boolHint() string {
	return "true/false, 1/0, yes/no; blank keeps runtime config/defaults."
}

type model struct {
	root        string
	shellEnv    string
	makeEnv     string
	values      map[string]string
	selected    int
	app         *tview.Application
	pages       *tview.Pages
	list        *tview.List
	form        *tview.Form
	status      *tview.TextView
	modalOpen   bool
	lastSavedAt string
}

func main() {
	rootFlag := flag.String("root", "", "repository root")
	envFlag := flag.String("env-file", "", "shell env file path")
	makeFlag := flag.String("make-env-file", "", "Makefile env file path")
	doctorFlag := flag.Bool("doctor", false, "print doctor output and exit")
	flag.Parse()

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dev-env-tui: %v\n", err)
		os.Exit(2)
	}
	m := &model{
		root:     root,
		shellEnv: resolvePath(root, *envFlag, ".env.dev"),
		makeEnv:  resolvePath(root, *makeFlag, ".env.dev.mk"),
		values:   make(map[string]string),
	}
	m.seedFromProcessEnv()
	if err := m.loadEnvFile(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load %s: %v\n", m.shellEnv, err)
	}
	m.applyRecommendedDefaults()

	if *doctorFlag {
		fmt.Print(stripTviewTags(m.doctorText()))
		return
	}

	if err := m.run(); err != nil {
		fmt.Fprintf(os.Stderr, "dev-env-tui: %v\n", err)
		os.Exit(1)
	}
}

func resolveRoot(raw string) (string, error) {
	if raw != "" {
		return filepath.Abs(raw)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if fileExists(filepath.Join(dir, "Makefile")) && fileExists(filepath.Join(dir, "scripts", "dev-env.sh")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return "", fmt.Errorf("could not find repo root; pass -root")
}

func resolvePath(root, raw, fallback string) string {
	if raw == "" {
		return filepath.Join(root, fallback)
	}
	if filepath.IsAbs(raw) {
		return raw
	}
	return filepath.Join(root, raw)
}

func (m *model) seedFromProcessEnv() {
	for _, key := range allKeys() {
		if value, ok := os.LookupEnv(key); ok && value != "" {
			m.values[key] = value
		}
	}
}

func (m *model) applyRecommendedDefaults() {
	defaults := map[string]string{
		"DISABLE_AUTH":                    "true",
		"GIN_MODE":                        "debug",
		"AGENT_EBPF_DEV_SESSION":          "agent-ebpf-dev",
		"DEV_CONTAINER":                   "agent-ebpf-filiter-dev",
		"DEV_WORKSPACE":                   "/workspaces/agent-ebpf-filiter",
		"DEVCONTAINER_POSTCREATE_INSTALL": "0",
	}
	if cudaDefault() != "" {
		defaults["CUDA_GO_TAGS"] = cudaDefault()
	}
	for key, value := range defaults {
		if strings.TrimSpace(m.values[key]) == "" {
			m.values[key] = value
		}
	}
	if strings.TrimSpace(m.values["CONTAINER_CLI"]) == "" {
		m.values["CONTAINER_CLI"] = detectContainerCLI()
	}
	m.applyDetectedDevcontainerDefaults()
}

func (m *model) applyDetectedDevcontainerDefaults() {
	branch := strings.TrimSpace(runGit(m.root, "branch", "--show-current"))
	ownerRepo := detectOwnerRepo(m.root)
	if strings.TrimSpace(m.values["DEV_BRANCH"]) == "" && branch != "" {
		m.values["DEV_BRANCH"] = branch
	}
	if strings.TrimSpace(m.values["DEV_IMAGE_REPOSITORY"]) == "" && ownerRepo != "" {
		m.values["DEV_IMAGE_REPOSITORY"] = "ghcr.io/" + ownerRepo + "/devcontainer"
	}
	if strings.TrimSpace(m.values["DEV_IMAGE_TAG"]) == "" && branch != "" {
		m.values["DEV_IMAGE_TAG"] = branchTag(branch)
	}
	if strings.TrimSpace(m.values["DEV_IMAGE"]) == "" && m.values["DEV_IMAGE_REPOSITORY"] != "" && m.values["DEV_IMAGE_TAG"] != "" {
		m.values["DEV_IMAGE"] = m.values["DEV_IMAGE_REPOSITORY"] + ":" + m.values["DEV_IMAGE_TAG"]
	}
}

func (m *model) loadEnvFile() error {
	if !fileExists(m.shellEnv) {
		return nil
	}
	var script strings.Builder
	script.WriteString("set -a\n. \"$DEV_ENV_TUI_FILE\"\nset +a\n")
	script.WriteString("for key in")
	for _, key := range allKeys() {
		script.WriteByte(' ')
		script.WriteString(key)
	}
	script.WriteString("; do printf '%s=%s\\000' \"$key\" \"${!key-}\"; done\n")
	cmd := exec.Command("bash", "-lc", script.String())
	cmd.Env = append(os.Environ(), "DEV_ENV_TUI_FILE="+m.shellEnv)
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	for _, part := range bytes.Split(out, []byte{0}) {
		if len(part) == 0 {
			continue
		}
		key, value, ok := strings.Cut(string(part), "=")
		if ok && value != "" {
			m.values[key] = value
		}
	}
	return nil
}

func (m *model) run() error {
	m.app = tview.NewApplication()
	m.pages = tview.NewPages()
	m.list = tview.NewList().ShowSecondaryText(true)
	m.list.SetBorder(true)
	m.list.SetTitle(" Groups  Ctrl+G ")
	m.form = tview.NewForm()
	m.form.SetBorder(true)
	m.status = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	m.status.SetBorder(true)
	m.status.SetTitle(" Status / Help ")

	for i, group := range groups {
		idx := i
		shortcut := rune(0)
		if i < 9 {
			shortcut = rune('1' + i)
		}
		m.list.AddItem(group.Title, group.Desc, shortcut, func() {
			m.selected = idx
			m.rebuildForm()
			m.app.SetFocus(m.form)
		})
	}
	m.rebuildForm()
	m.setStatus("[green]Ready.[white] Ctrl+S save, Ctrl+D doctor, Ctrl+P preview, Ctrl+G groups, Ctrl+F form, Ctrl+Q quit. Empty field = unset.")

	header := tview.NewTextView().SetDynamicColors(true)
	header.SetText("[::b]Agent eBPF Filter Dev Env TUI[::-]\n[gray]Edit .env.dev / .env.dev.mk for local development, LLM, and application behavior.[-]")
	header.SetBorder(true)

	mainFlex := tview.NewFlex().
		AddItem(m.list, 32, 1, true).
		AddItem(m.form, 0, 3, false)
	rootFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 4, 0, false).
		AddItem(mainFlex, 0, 1, true).
		AddItem(m.status, 7, 0, false)

	m.pages.AddPage("main", rootFlex, true, true)
	m.app.SetRoot(m.pages, true)
	m.app.SetFocus(m.list)
	m.app.SetInputCapture(m.captureKey)
	return m.app.Run()
}

func (m *model) captureKey(event *tcell.EventKey) *tcell.EventKey {
	if m.modalOpen && event.Key() == tcell.KeyEsc {
		m.closeModal()
		return nil
	}
	switch event.Key() {
	case tcell.KeyCtrlS:
		m.saveFromUI()
		return nil
	case tcell.KeyCtrlD:
		m.showModal("Doctor", m.doctorText())
		return nil
	case tcell.KeyCtrlP:
		m.showModal("Preview", m.previewText(true))
		return nil
	case tcell.KeyCtrlQ:
		m.app.Stop()
		return nil
	case tcell.KeyCtrlG:
		m.app.SetFocus(m.list)
		return nil
	case tcell.KeyCtrlF:
		m.app.SetFocus(m.form)
		return nil
	}
	return event
}

func (m *model) rebuildForm() {
	group := groups[m.selected]
	m.form.Clear(true)
	m.form.SetTitle(" " + group.Title + "  Ctrl+F ")
	for _, item := range group.Vars {
		item := item
		label := fmt.Sprintf("%s (%s)", item.Label, item.Key)
		value := m.values[item.Key]
		if item.Secret {
			m.form.AddPasswordField(label, value, 48, '*', func(text string) {
				m.setValue(item.Key, text)
			})
		} else {
			m.form.AddInputField(label, value, 48, nil, func(text string) {
				m.setValue(item.Key, text)
			})
		}
	}
	m.form.AddButton("Save", m.saveFromUI)
	m.form.AddButton("Doctor", func() { m.showModal("Doctor", m.doctorText()) })
	m.form.AddButton("Preview", func() { m.showModal("Preview", m.previewText(true)) })
	m.form.AddButton("Quit", func() { m.app.Stop() })
	m.form.SetButtonsAlign(tview.AlignRight)
	m.setStatus(fmt.Sprintf("[yellow]%s[white]: %s\n[gray]Tip: %s[-]", group.Title, group.Desc, firstHint(group)))
}

func (m *model) setValue(key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		delete(m.values, key)
		return
	}
	m.values[key] = value
}

func (m *model) saveFromUI() {
	if err := m.writeFiles(); err != nil {
		m.setStatus("[red]Save failed:[white] " + err.Error())
		return
	}
	m.lastSavedAt = time.Now().Format("15:04:05")
	m.setStatus(fmt.Sprintf("[green]Saved at %s.[white] %s and %s", m.lastSavedAt, rel(m.root, m.shellEnv), rel(m.root, m.makeEnv)))
}

func (m *model) setStatus(text string) {
	m.status.SetText(text)
}

func (m *model) showModal(title, content string) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(false)
	view.SetBorder(true)
	view.SetTitle(" " + title + "  Esc closes ")
	view.SetText(content)
	view.SetDoneFunc(func(key tcell.Key) {
		m.closeModal()
	})
	m.modalOpen = true
	m.pages.AddPage("modal", centered(view, 90, 28), true, true)
	m.app.SetFocus(view)
}

func (m *model) closeModal() {
	m.pages.RemovePage("modal")
	m.modalOpen = false
	m.app.SetFocus(m.form)
}

func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}

func (m *model) writeFiles() error {
	if err := os.MkdirAll(filepath.Dir(m.shellEnv), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.makeEnv), 0o755); err != nil {
		return err
	}
	generated := time.Now().UTC().Format(time.RFC3339)
	var shellOut strings.Builder
	shellOut.WriteString(fmt.Sprintf("# Generated by tools/dev-env-tui at %s\n", generated))
	shellOut.WriteString("# Source this file for shell sessions: set -a; . ./.env.dev; set +a\n")
	for _, group := range groups {
		shellOut.WriteString("\n# " + group.Title + "\n")
		for _, item := range group.Vars {
			value := strings.TrimSpace(m.values[item.Key])
			if value == "" {
				continue
			}
			shellOut.WriteString(fmt.Sprintf("export %s=%s\n", item.Key, shellQuote(value)))
		}
	}

	var makeOut strings.Builder
	makeOut.WriteString(fmt.Sprintf("# Generated by tools/dev-env-tui at %s\n", generated))
	makeOut.WriteString("# Included automatically by Makefile when present.\n")
	for _, group := range groups {
		makeOut.WriteString("\n# " + group.Title + "\n")
		for _, item := range group.Vars {
			value := strings.TrimSpace(m.values[item.Key])
			if value == "" {
				continue
			}
			makeOut.WriteString(fmt.Sprintf("%s := %s\n", item.Key, makeEscape(value)))
			makeOut.WriteString(fmt.Sprintf("export %s\n", item.Key))
		}
	}
	if err := os.WriteFile(m.shellEnv, []byte(shellOut.String()), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(m.shellEnv, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(m.makeEnv, []byte(makeOut.String()), 0o600); err != nil {
		return err
	}
	return os.Chmod(m.makeEnv, 0o600)
}

func (m *model) doctorText() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[::b]Dev env files[::-]\n  shell: %s\n  make : %s\n\n", existsLabel(m.shellEnv, m.root), existsLabel(m.makeEnv, m.root)))
	b.WriteString(m.previewText(true))
	b.WriteString("\n[::b]Config checks[::-]\n")
	apiKey, accessToken := m.values["AGENT_API_KEY"], m.values["AGENT_ACCESS_TOKEN"]
	if apiKey != "" && accessToken != "" && apiKey != accessToken {
		b.WriteString("  [yellow]warn[-]: AGENT_API_KEY and AGENT_ACCESS_TOKEN differ; backend prefers AGENT_API_KEY.\n")
	}
	llmEnabled := isTruthy(m.values["AGENT_LLM_ENABLED"])
	llmBase := firstNonEmpty(m.values["AGENT_LLM_BASE_URL"], m.values["OPENAI_BASE_URL"])
	llmModel := firstNonEmpty(m.values["AGENT_LLM_MODEL"], m.values["OPENAI_MODEL"])
	llmKey := firstNonEmpty(m.values["AGENT_LLM_API_KEY"], m.values["OPENAI_API_KEY"])
	if llmEnabled {
		if llmBase == "" || llmModel == "" {
			b.WriteString("  [yellow]warn[-]: AGENT_LLM_ENABLED is true but base URL or model is unset.\n")
		} else {
			b.WriteString("  [green]ok[-]  : LLM override has base URL and model.\n")
		}
	} else if llmBase != "" || llmModel != "" || llmKey != "" {
		b.WriteString("  [blue]note[-]: LLM values are present; set AGENT_LLM_ENABLED=true to force-enable at backend startup.\n")
	} else {
		b.WriteString("  [green]ok[-]  : no LLM env override; Runtime Config remains source of truth.\n")
	}
	if isTruthy(m.values["AGENT_EBPF_NO_SANDBOX"]) && isTruthy(m.values["AGENT_EBPF_SANDBOX_STRICT"]) {
		b.WriteString("  [yellow]warn[-]: sandbox is disabled while strict mode is also requested.\n")
	}
	if m.values["AGENT_CLUSTER_MASTER_URL"] != "" && (m.values["AGENT_CLUSTER_ACCOUNT"] == "" || m.values["AGENT_CLUSTER_PASSWORD"] == "") {
		b.WriteString("  [yellow]warn[-]: cluster master URL is set but account/password is incomplete.\n")
	}
	b.WriteString("\n[::b]Tool checks[::-]\n")
	for _, name := range []string{"go", "make", "bun", "uv", "protoc", "zellij", "sudo"} {
		b.WriteString(toolLine(name, name))
	}
	container := m.values["CONTAINER_CLI"]
	if container == "" {
		container = detectContainerCLI()
	}
	b.WriteString(toolLine("container", container))
	return b.String()
}

func (m *model) previewText(redact bool) string {
	var b strings.Builder
	for _, group := range groups {
		b.WriteString(fmt.Sprintf("[::b][%s][::-]\n", group.Title))
		count := 0
		for _, item := range group.Vars {
			value := strings.TrimSpace(m.values[item.Key])
			if value == "" {
				continue
			}
			if redact && item.Secret {
				value = redactValue(value)
			}
			b.WriteString(fmt.Sprintf("  %-42s %s\n", item.Key, value))
			count++
		}
		if count == 0 {
			b.WriteString("  (no values set)\n")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func allKeys() []string {
	keys := make([]string, 0, 128)
	for _, group := range groups {
		for _, item := range group.Vars {
			keys = append(keys, item.Key)
		}
	}
	return keys
}

func firstHint(group envGroup) string {
	for _, item := range group.Vars {
		if item.Hint != "" {
			return item.Key + ": " + item.Hint
		}
	}
	return "Empty fields are not written."
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func makeEscape(value string) string {
	value = strings.ReplaceAll(value, "$", "$$")
	value = strings.ReplaceAll(value, "#", `\#`)
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func redactValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "[set]"
	}
	return value[:4] + "…" + value[len(value)-4:]
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func toolLine(label, cmdName string) string {
	if strings.TrimSpace(cmdName) == "" {
		return fmt.Sprintf("  %-10s [yellow]missing[-]\n", label)
	}
	if path, err := exec.LookPath(cmdName); err == nil {
		return fmt.Sprintf("  %-10s [green]%s[-]\n", label, path)
	}
	return fmt.Sprintf("  %-10s [yellow]missing %s[-]\n", label, cmdName)
}

func existsLabel(path, root string) string {
	if fileExists(path) {
		return rel(root, path)
	}
	return "missing (" + rel(root, path) + ")"
}

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectContainerCLI() string {
	if path, err := exec.LookPath("docker"); err == nil {
		return path
	}
	if path, err := exec.LookPath("podman"); err == nil {
		return path
	}
	return ""
}

func cudaDefault() string {
	if fileExists("/opt/cuda/bin/nvcc") && fileExists("/opt/cuda/lib64/libcudart.so") {
		return "cuda"
	}
	return ""
}

func branchTag(branch string) string {
	base := sanitizeTagBase(branch)
	if base == "" {
		base = "local"
	}
	sum := sha256.Sum256([]byte(branch))
	suffix := hex.EncodeToString(sum[:])[:12]
	if len(base) > 115 {
		base = strings.TrimRight(base[:115], "-")
	}
	if base == "" {
		base = "local"
	}
	return base + "-" + suffix
}

func sanitizeTagBase(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func runGit(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func detectOwnerRepo(root string) string {
	remote := runGit(root, "remote", "get-url", "origin")
	remote = strings.TrimSpace(remote)
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "ssh://git@github.com/"} {
		remote = strings.TrimPrefix(remote, prefix)
	}
	remote = strings.TrimSuffix(remote, ".git")
	return strings.ToLower(strings.Trim(remote, "/"))
}

func stripTviewTags(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '[' {
			if end := strings.IndexByte(value[i:], ']'); end >= 0 {
				tag := value[i+1 : i+end]
				if tag == "-" || strings.Contains(tag, ":") || tag == "green" || tag == "yellow" || tag == "blue" || tag == "red" || tag == "gray" || tag == "white" {
					i += end
					continue
				}
			}
		}
		b.WriteByte(value[i])
	}
	return b.String()
}
