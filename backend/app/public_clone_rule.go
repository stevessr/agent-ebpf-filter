package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent-ebpf-filter/pb"
)

const (
	publicCloneProcessTTL = 90 * time.Second
	publicCloneCacheTTL   = 60 * time.Second
	publicCloneCacheLimit = 256
)

var (
	githubCloneName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	githubCloneHTTP = &http.Client{Timeout: 900 * time.Millisecond}
	publicCloneCache = struct {
		sync.Mutex
		entries map[string]time.Time
	}{entries: make(map[string]time.Time)}
	// Process observations are read from the hot event path. Publish an
	// immutable map snapshot on the rare registration write, not a mutex
	// acquisition for every kernel event.
	publicCloneProcesses = struct {
		sync.Mutex
		entries atomic.Pointer[map[uint32]publicCloneProcess]
	}{}
)

type publicCloneProcess struct {
	comm    string
	expires time.Time
}

// publicGitHubCloneRepository recognizes only simple, unauthenticated GitHub
// HTTPS clones. Command intent alone does NOT establish public visibility;
// checkPublicGitHubRepository performs an anonymous visibility check.
func publicGitHubCloneRepository(comm string, args []string) (string, bool) {
	comm = strings.ToLower(filepath.Base(strings.TrimSpace(comm)))
	switch comm {
	case "git":
		if len(args) < 2 || args[0] != "clone" {
			return "", false
		}
		// Git accepts options before its remote operand. Only a small set
		// of transfer/read-only options is safe to recognize automatically.
		index := 1
		for index < len(args) && strings.HasPrefix(args[index], "-") {
			n, ok := safeGitCloneOption(args[index:])
			if !ok {
				return "", false
			}
			index += n
		}
		if len(args)-index < 1 || len(args)-index > 2 {
			return "", false
		}
		if len(args)-index == 2 && !safeCloneDestination(args[index+1]) {
			return "", false
		}
		return httpsGitHubCloneRepository(args[index])
	case "gh":
		if len(args) < 3 || args[0] != "repo" || args[1] != "clone" {
			return "", false
		}
		repo, ok := githubCloneRepoPath("/" + args[2])
		if strings.HasPrefix(args[2], "https://") {
			repo, ok = httpsGitHubCloneRepository(args[2])
		}
		if !ok {
			return "", false
		}
		rest := args[3:]
		if len(rest) != 0 && rest[0] != "--" {
			if !safeCloneDestination(rest[0]) {
				return "", false
			}
			rest = rest[1:]
		}
		if len(rest) == 0 {
			return repo, true
		}
		if rest[0] != "--" || len(rest) < 2 {
			return "", false
		}
		// gh passes arguments after -- to git clone. Only approved options
		// are allowed, never another repository or a command override.
		rest = rest[1:]
		for len(rest) > 0 {
			n, valid := safeGitCloneOption(rest)
			if !valid {
				return "", false
			}
			rest = rest[n:]
		}
		return repo, true
	default:
		return "", false
	}
}

func httpsGitHubCloneRepository(raw string) (string, bool) {
	remote, err := url.Parse(raw)
	if err != nil || remote.Scheme != "https" || !strings.EqualFold(remote.Host, "github.com") ||
		remote.User != nil || remote.RawQuery != "" || remote.Fragment != "" ||
		remote.Opaque != "" || remote.RawPath != "" || strings.Contains(raw, "%") {
		return "", false
	}
	return githubCloneRepoPath(remote.Path)
}

// These flags affect clone transport/output, not hooks, executable helpers,
// remote URLs, or arbitrary Git configuration. No other flag receives the
// built-in reduction in loop-alert sensitivity.
func safeGitCloneOption(args []string) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "-q", "--quiet", "--progress", "--single-branch", "--no-checkout":
		return 1, true
	case "--depth":
		if len(args) > 1 && validCloneDepth(args[1]) {
			return 2, true
		}
	case "-b", "--branch":
		if len(args) > 1 && safeCloneBranch(args[1]) {
			return 2, true
		}
	}
	if strings.HasPrefix(args[0], "--depth=") && validCloneDepth(strings.TrimPrefix(args[0], "--depth=")) {
		return 1, true
	}
	if strings.HasPrefix(args[0], "--branch=") && safeCloneBranch(strings.TrimPrefix(args[0], "--branch=")) {
		return 1, true
	}
	if args[0] == "--filter=blob:none" {
		return 1, true
	}
	return 0, false
}

func validCloneDepth(raw string) bool {
	if len(raw) == 0 || len(raw) > 7 {
		return false
	}
	var n int
	for _, c := range raw {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n > 0 && n <= 1_000_000
}

func safeCloneBranch(value string) bool {
	if len(value) == 0 || len(value) > 128 || strings.HasPrefix(value, "-") ||
		strings.Contains(value, "..") || strings.HasSuffix(value, ".lock") ||
		strings.HasSuffix(value, "/") || strings.HasPrefix(value, "/") {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '/' || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func safeCloneDestination(dst string) bool {
	return dst != "" && filepath.IsLocal(dst) && !strings.HasPrefix(dst, "-")
}

func githubCloneRepoPath(path string) (string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		return "", false
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if !githubCloneName.MatchString(owner) || !githubCloneName.MatchString(repo) ||
		owner == "." || owner == ".." || repo == "." || repo == ".." {
		return "", false
	}
	return owner + "/" + repo, true
}

// Verification is deliberately fail-closed and unauthenticated: a private repo
// accessible through the local gh credentials must not count as public.
func isPublicGitHubRepository(ctx context.Context, repo string) bool {
	now := time.Now()
	publicCloneCache.Lock()
	expiry, cached := publicCloneCache.entries[strings.ToLower(repo)]
	publicCloneCache.Unlock()
	if cached && now.Before(expiry) {
		return true
	}
	ok := checkPublicGitHubRepository(ctx, githubCloneHTTP, "https://api.github.com", repo)
	if ok {
		publicCloneCache.Lock()
		if len(publicCloneCache.entries) >= publicCloneCacheLimit {
			clear(publicCloneCache.entries)
		}
		publicCloneCache.entries[strings.ToLower(repo)] = now.Add(publicCloneCacheTTL)
		publicCloneCache.Unlock()
	}
	return ok
}

func checkPublicGitHubRepository(ctx context.Context, client *http.Client, apiBase, repo string) bool {
	if client == nil || ctx == nil {
		return false
	}
	url := strings.TrimRight(apiBase, "/") + "/repos/" + repo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var metadata struct {
		FullName   string `json:"full_name"`
		Private    *bool  `json:"private"`
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&metadata); err != nil {
		return false
	}
	return strings.EqualFold(metadata.FullName, repo) && metadata.Private != nil &&
		!*metadata.Private && (metadata.Visibility == "" || metadata.Visibility == "public")
}

func markPublicCloneProcess(pid uint32, comm string) {
	if pid == 0 {
		return
	}
	publicCloneProcesses.Lock()
	defer publicCloneProcesses.Unlock()
	now := time.Now()
	next := make(map[uint32]publicCloneProcess)
	if previous := publicCloneProcesses.entries.Load(); previous != nil {
		for oldPID, entry := range *previous {
			if now.Before(entry.expires) {
				next[oldPID] = entry
			}
		}
	}
	if len(next) >= publicCloneCacheLimit {
		clear(next)
	}
	next[pid] = publicCloneProcess{
		comm: strings.ToLower(filepath.Base(comm)), expires: now.Add(publicCloneProcessTTL),
	}
	publicCloneProcesses.entries.Store(&next)
}

func unmarkPublicCloneProcess(pid uint32) {
	publicCloneProcesses.Lock()
	defer publicCloneProcesses.Unlock()
	old := publicCloneProcesses.entries.Load()
	if old == nil {
		return
	}
	next := make(map[uint32]publicCloneProcess, len(*old))
	for previousPID, entry := range *old {
		if previousPID != pid {
			next[previousPID] = entry
		}
	}
	publicCloneProcesses.entries.Store(&next)
}

// A verified clone raises ONLY loop heuristics for the launching process and
// its direct Git transport children, not secret access or other security checks.
func isVerifiedPublicCloneProcess(event *pb.Event) bool {
	if event == nil {
		return false
	}
	snapshot := publicCloneProcesses.entries.Load()
	if snapshot == nil {
		return false
	}
	comm := strings.ToLower(filepath.Base(strings.TrimSpace(event.GetComm())))
	now := time.Now()
	if entry, ok := (*snapshot)[event.GetPid()]; ok &&
		now.Before(entry.expires) && entry.comm == comm {
		// Remove process trust as soon as the monitored PID exits to
		// avoid inheriting an exemption if Linux reuses the PID.
		if event.GetType() == "process_exit" || event.GetType() == "exit" {
			unmarkPublicCloneProcess(event.GetPid())
			return false
		}
		return true
	}
	if entry, ok := (*snapshot)[event.GetPpid()]; ok && now.Before(entry.expires) {
		switch comm {
		case "git", "git-remote-http", "git-remote-https", "git-index-pack":
			return true
		}
	}
	return false
}
