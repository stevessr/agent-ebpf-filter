package app

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type agentLegalDatasetRequest struct {
	Limit  int  `json:"limit"`
	Import bool `json:"import"`
}

type agentLegalBehaviorTemplate struct {
	Family      string
	Description string
	CommandLine string
	Comm        string
	Args        []string
}

type agentLegalDatasetResponse struct {
	Source         string                     `json:"source"`
	Format         string                     `json:"format"`
	ContentType    string                     `json:"contentType"`
	Total          int                        `json:"total"`
	Limit          int                        `json:"limit"`
	Truncated      bool                       `json:"truncated"`
	Imported       int                        `json:"imported,omitempty"`
	Skipped        int                        `json:"skipped,omitempty"`
	TotalSamples   int                        `json:"totalSamples,omitempty"`
	LabeledSamples int                        `json:"labeledSamples,omitempty"`
	ByLabel        []researchCount            `json:"byLabel,omitempty"`
	ByCategory     []researchCount            `json:"byCategory,omitempty"`
	BySource       []researchCount            `json:"bySource,omitempty"`
	Rows           []remoteDatasetRow         `json:"rows,omitempty"`
	Families       map[string]int             `json:"families,omitempty"`
	Normalization  FeatureNormalizationReport `json:"normalization"`
	Quality        DatasetQualitySummary      `json:"quality,omitempty"`
}

func handleMLAgentLegalDatasetPost(c *gin.Context) {
	var req agentLegalDatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ML training store not initialized"})
		return
	}

	resp, samples := buildAgentLegalDatasetResponse(req.Limit)
	if req.Import {
		imported, skipped, err := importAgentLegalSamples(samples)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp.Imported = imported
		resp.Skipped += skipped
		total, labeled := globalTrainingStore.Status()
		resp.TotalSamples = total
		resp.LabeledSamples = labeled
		log.Printf("[ML] Builtin dataset import source=%q rows=%d imported=%d skipped=%d", resp.Source, len(resp.Rows), imported, resp.Skipped)
	}
	c.JSON(http.StatusOK, resp)
}

func buildAgentLegalDatasetResponse(limit int) (agentLegalDatasetResponse, []TrainingSample) {
	templates := builtinAgentLegalBehaviorTemplates()
	if limit <= 0 || limit > len(templates) {
		limit = len(templates)
	}

	now := time.Now().UTC()
	rows := make([]remoteDatasetRow, 0, limit)
	samples := make([]TrainingSample, 0, limit)
	families := make(map[string]int)
	skipped := 0

	for i, tmpl := range templates[:limit] {
		if tmpl.Comm == "" {
			skipped++
			continue
		}
		sample := buildAgentLegalTrainingSample(tmpl, now.Add(time.Duration(i)*time.Millisecond))
		row := trainingSampleToRemoteDatasetRow(i+1, sample)
		row.Source = tmpl.Family
		row.LabelSource = sample.UserLabel
		row.Duplicate = globalTrainingStore != nil && globalTrainingStore.HasExactCommand(sample.Comm, sample.Args)
		rows = append(rows, row)
		samples = append(samples, sample)
		families[tmpl.Family]++
	}

	normalization := summarizeFeatureNormalization(samples)
	statResp := remoteDatasetResponse{Source: "builtin-agent-legal-behavior", Rows: rows, Normalization: normalization}
	applyRemoteDatasetResponseStats(&statResp, "preserve", false)

	resp := agentLegalDatasetResponse{
		Source:        "builtin-agent-legal-behavior",
		Format:        "builtin",
		ContentType:   "application/json",
		Total:         len(rows),
		Limit:         limit,
		Truncated:     limit < len(templates),
		Skipped:       skipped,
		ByLabel:       statResp.ByLabel,
		ByCategory:    statResp.ByCategory,
		BySource:      statResp.BySource,
		Rows:          rows,
		Families:      families,
		Normalization: normalization,
		Quality:       statResp.Quality,
	}
	return resp, samples
}

func importAgentLegalSamples(samples []TrainingSample) (int, int, error) {
	imported := 0
	skipped := 0
	seen := make(map[string]struct{})
	for _, sample := range samples {
		if sample.Comm == "" {
			skipped++
			continue
		}
		key := commandKey(sample.Comm, sample.Args)
		if _, ok := seen[key]; ok {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		if globalTrainingStore.HasExactCommand(sample.Comm, sample.Args) {
			skipped++
			continue
		}
		globalTrainingStore.Add(sample)
		recordCommandSampleSideEffects(sample)
		imported++
	}
	if err := globalTrainingStore.Flush(); err != nil {
		return imported, skipped, err
	}
	return imported, skipped, nil
}

func buildAgentLegalTrainingSample(tmpl agentLegalBehaviorTemplate, timestamp time.Time) TrainingSample {
	sample := buildCommandTrainingSample(tmpl.Comm, tmpl.Args, "", 0, 0, "agent-legal", timestamp)
	sample.CommandLine = tmpl.CommandLine
	return sample
}

func builtinAgentLegalBehaviorTemplates() []agentLegalBehaviorTemplate {
	commands := []struct {
		family      string
		description string
		commandLine string
	}{
		// Repository inspection.
		{"git-readonly", "inspect repository status", "git status --short"},
		{"git-readonly", "inspect current branch", "git branch --show-current"},
		{"git-readonly", "inspect remotes", "git remote -v"},
		{"git-readonly", "inspect recent commits", "git log --oneline -n 20"},
		{"git-readonly", "inspect diff", "git diff --stat"},
		{"git-readonly", "inspect staged diff", "git diff --cached --stat"},
		{"git-readonly", "inspect file history", "git blame README.md"},
		{"git-readonly", "fetch metadata", "git fetch --all --prune"},
		{"git-readonly", "list tracked files", "git ls-files"},
		{"git-readonly", "show commit summary", "git show --stat HEAD"},

		// Filesystem read/search patterns common for coding agents.
		{"file-read-search", "print working directory", "pwd"},
		{"file-read-search", "list source directory", "ls -la"},
		{"file-read-search", "list docs", "ls -la docs"},
		{"file-read-search", "read README", "cat README.md"},
		{"file-read-search", "read package manifest", "cat package.json"},
		{"file-read-search", "read go module", "cat go.mod"},
		{"file-read-search", "preview source", "sed -n 1,160p backend/app/main.go"},
		{"file-read-search", "preview docs", "head -n 80 README.md"},
		{"file-read-search", "tail log", "tail -n 200 frontend/build.log"},
		{"file-read-search", "find TypeScript files", "find frontend/src -name *.ts -maxdepth 4"},
		{"file-read-search", "find Vue components", "find frontend/src -name *.vue -maxdepth 5"},
		{"file-read-search", "grep TODO markers", "grep -RIn TODO backend/app"},
		{"file-read-search", "ripgrep symbols", "rg AutoTune backend/app"},
		{"file-read-search", "count lines", "wc -l README.md"},
		{"file-read-search", "check disk use", "du -sh ."},

		// Build and test commands.
		{"build-test-go", "go test package", "go test ./app"},
		{"build-test-go", "go test core and ml", "go test ./core ./ml"},
		{"build-test-go", "go test all packages", "go test ./..."},
		{"build-test-go", "go build backend", "go build ./app"},
		{"build-test-go", "go vet scoped", "go vet ./app"},
		{"build-test-go", "go list modules", "go list ./..."},
		{"build-test-go", "go mod download", "go mod download"},
		{"build-test-go", "go fmt check", "gofmt -w backend/app/auto_tune.go"},
		{"build-test-js", "install frontend deps", "bun install"},
		{"build-test-js", "build frontend", "bun run build"},
		{"build-test-js", "run vite dev", "bun run dev"},
		{"build-test-js", "type check", "bunx vue-tsc --noEmit"},
		{"build-test-js", "npm test", "npm test"},
		{"build-test-js", "pnpm install frozen", "pnpm install --frozen-lockfile"},
		{"build-test-js", "lint frontend", "npm run lint"},
		{"build-test-python", "run pytest", "python3 -m pytest"},
		{"build-test-python", "compile python", "python3 -m py_compile scripts/analyze_codex_stripped.py"},
		{"build-test-python", "run ruff", "python3 -m ruff check ."},
		{"build-test-python", "install editable", "uv pip install -e adapters/python"},
		{"build-test-rust", "cargo check", "cargo check"},
		{"build-test-rust", "cargo test", "cargo test"},
		{"build-test-rust", "cargo fmt check", "cargo fmt --check"},
		{"build-test-java", "maven test", "mvn test"},
		{"build-test-java", "gradle test", "gradle test"},

		// Safe local file writes that agents often perform during builds.
		{"safe-file-write", "make build directory", "mkdir -p dist"},
		{"safe-file-write", "make tmp directory", "mkdir -p /tmp/agent-ebpf-filter"},
		{"safe-file-write", "copy docs to temp", "cp README.md /tmp/agent-ebpf-filter/README.md"},
		{"safe-file-write", "touch temp marker", "touch /tmp/agent-ebpf-filter/ready"},
		{"safe-file-write", "write temp note", "tee /tmp/agent-ebpf-filter/note.txt"},
		{"safe-file-write", "archive listing", "tar -tf backup.tar.gz"},
		{"safe-file-write", "create project archive", "tar -czf /tmp/agent-ebpf-filter/docs.tgz docs"},
		{"safe-file-write", "zip docs", "zip -r /tmp/agent-ebpf-filter/docs.zip docs"},
		{"safe-file-write", "remove temp build dir", "rm -rf /tmp/agent-ebpf-filter/build"},
		{"safe-file-write", "remove node_modules cache", "rm -rf node_modules/.cache"},

		// Package and environment introspection.
		{"env-introspection", "node version", "node --version"},
		{"env-introspection", "bun version", "bun --version"},
		{"env-introspection", "go version", "go version"},
		{"env-introspection", "python version", "python3 --version"},
		{"env-introspection", "rust version", "rustc --version"},
		{"env-introspection", "print env path", "printenv PATH"},
		{"env-introspection", "whoami", "whoami"},
		{"env-introspection", "uname", "uname -a"},
		{"env-introspection", "disk free", "df -h"},
		{"env-introspection", "memory free", "free -h"},
		{"env-introspection", "process list", "ps aux"},
		{"env-introspection", "network sockets", "ss -tulpn"},

		// Benign network lookups used for dependency metadata and public APIs.
		{"network-benign", "curl public api", "curl https://api.github.com/repos/openai/openai"},
		{"network-benign", "curl docs", "curl https://example.com"},
		{"network-benign", "wget readme", "wget https://example.com/robots.txt -O /tmp/robots.txt"},
		{"network-benign", "dns lookup", "dig github.com"},
		{"network-benign", "host lookup", "host github.com"},
		{"network-benign", "ping gateway", "ping -c 1 127.0.0.1"},
		{"network-benign", "npm view", "npm view vue version"},
		{"network-benign", "pip index", "python3 -m pip index versions pytest"},

		// Container and orchestration read-only workflows.
		{"container-readonly", "docker ps", "docker ps"},
		{"container-readonly", "docker images", "docker images"},
		{"container-readonly", "docker inspect", "docker inspect agent-ebpf-filter"},
		{"container-readonly", "podman ps", "podman ps"},
		{"container-readonly", "kubectl get pods", "kubectl get pods"},
		{"container-readonly", "kubectl get nodes", "kubectl get nodes"},
		{"container-readonly", "kubectl describe", "kubectl describe pod app"},
		{"container-readonly", "kubectl logs", "kubectl logs deploy/app"},
		{"container-readonly", "helm list", "helm list"},

		// Database and service read-only checks.
		{"service-readonly", "sqlite schema", "sqlite3 app.db .schema"},
		{"service-readonly", "psql version", "psql --version"},
		{"service-readonly", "mysql version", "mysql --version"},
		{"service-readonly", "redis ping", "redis-cli ping"},
		{"service-readonly", "systemd status", "systemctl status agent-ebpf-filter"},
		{"service-readonly", "journal tail", "journalctl -u agent-ebpf-filter -n 100 --no-pager"},
		{"service-readonly", "lsof backend", "lsof -i :8080"},
		{"service-readonly", "bpftool show", "bpftool prog list"},
		{"service-readonly", "list bpf maps", "bpftool map list"},
		{"service-readonly", "read cgroup mounts", "mount | grep cgroup2"},
	}

	out := make([]agentLegalBehaviorTemplate, 0, len(commands))
	for _, item := range commands {
		parts := splitCommandLine(item.commandLine)
		if len(parts) == 0 {
			continue
		}
		out = append(out, agentLegalBehaviorTemplate{
			Family:      item.family,
			Description: item.description,
			CommandLine: item.commandLine,
			Comm:        parts[0],
			Args:        parts[1:],
		})
	}
	return out
}
