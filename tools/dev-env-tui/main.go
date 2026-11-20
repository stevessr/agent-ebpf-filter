package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	themeBg        = tcell.NewRGBColor(0x07, 0x12, 0x1f)
	themePanel     = tcell.NewRGBColor(0x0f, 0x1b, 0x2d)
	themePanelAlt  = tcell.NewRGBColor(0x1e, 0x29, 0x3b)
	themeBorder    = tcell.NewRGBColor(0x38, 0xbd, 0xf8)
	themeTitle     = tcell.NewRGBColor(0x7d, 0xd3, 0xfc)
	themeText      = tcell.NewRGBColor(0xe2, 0xe8, 0xf0)
	themeMuted     = tcell.NewRGBColor(0x94, 0xa3, 0xb8)
	themeAccent    = tcell.NewRGBColor(0x22, 0xd3, 0xee)
	themeAccentDim = tcell.NewRGBColor(0x0e, 0x74, 0x9)
	themeButton    = tcell.NewRGBColor(0x25, 0x63, 0xeb)
	themeWarning   = tcell.NewRGBColor(0xfb, 0xbf, 0x24)
	themeSuccess   = tcell.NewRGBColor(0x22, 0xc5, 0x5e)
	themeError     = tcell.NewRGBColor(0xf8, 0x71, 0x71)
)

func applyTheme() {
	tview.Styles.PrimitiveBackgroundColor = themeBg
	tview.Styles.ContrastBackgroundColor = themePanelAlt
	tview.Styles.MoreContrastBackgroundColor = themeAccentDim
	tview.Styles.BorderColor = themeBorder
	tview.Styles.TitleColor = themeTitle
	tview.Styles.GraphicsColor = themeBorder
	tview.Styles.PrimaryTextColor = themeText
	tview.Styles.SecondaryTextColor = themeMuted
	tview.Styles.TertiaryTextColor = themeAccent
	tview.Styles.InverseTextColor = themeBg
	tview.Styles.ContrastSecondaryTextColor = themeText
}

func styleBox(box *tview.Box) {
	box.SetBackgroundColor(themePanel)
	box.SetBorderColor(themeBorder)
	box.SetTitleColor(themeTitle)
}

func colorTag(color tcell.Color) string {
	r, g, b := color.RGB()
	return fmt.Sprintf("[#%02x%02x%02x]", r, g, b)
}

func resetTag() string { return "[-]" }

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
			{Key: "AGENT_RUNTIME_LOG_FILE_PATH", Label: "Event store path", Hint: "Default: ~/.config/agent-ebpf-filter/events.pebble; .jsonl selects legacy mode."},
			{Key: "AGENT_RUNTIME_EVENT_STORE_MAX_RECORDS", Label: "Event DB max records", Hint: "Default: 250000."},
			{Key: "AGENT_RUNTIME_EVENT_STORE_MAX_AGE", Label: "Event DB max age", Hint: "Default: 168h; 0 disables age pruning."},
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
		Desc:  "Build features, CUDA, smoke tests, runtime replay, ML sweep",
		Vars: []envVar{
			{Key: "AGENT_BUILD_FEATURES", Label: "Backend features", Hint: "all, core, or comma-separated names such as tls_capture,ml,plugins."},
			{Key: "AGENT_BUILD_TAGS", Label: "Extra Go tags", Hint: "Advanced: raw Go build tags appended after feature and CUDA tags."},
			{Key: "AGENT_FRONTEND_BUILD_FEATURES", Label: "Frontend features", Hint: "Usually same as AGENT_BUILD_FEATURES; exported as VITE_AGENT_BUILD_FEATURES."},
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
