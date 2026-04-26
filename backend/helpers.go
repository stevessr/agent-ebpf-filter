package main

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const hookMarker = "agent-ebpf-hook-active"
const kiroManagedAgent = "agent-ebpf-hook"
const (
	textPreviewLimitBytes   = 64 * 1024
	binaryPreviewLimitBytes = 4 * 1024
	imagePreviewLimitBytes  = 2 * 1024 * 1024
)

var (
	getRealHomeOnce sync.Once
	getRealHomeVal  string
)

func getRealHomeDir() string {
	getRealHomeOnce.Do(func() {
		// 1. Check for our own environment variable (passed across sudo/pkexec)
		if h := os.Getenv("AGENT_REAL_HOME"); h != "" {
			getRealHomeVal = h
			return
		}
		// 2. If we are root, try to find the real user who started us via standard envs
		if os.Getuid() == 0 {
			// Try sudo user
			if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
				if u, err := user.Lookup(sudoUser); err == nil {
					getRealHomeVal = u.HomeDir
					return
				}
			}
			// Try pkexec user (PolicyKit)
			if pkexecUid := os.Getenv("PKEXEC_UID"); pkexecUid != "" {
				if u, err := user.LookupId(pkexecUid); err == nil {
					getRealHomeVal = u.HomeDir
					return
				}
			}
			// Try preserved HOME if it's not /root
			if home := os.Getenv("HOME"); home != "" && home != "/root" {
				getRealHomeVal = home
				return
			}
			// Try to find the first non-root user in /home
			if entries, err := os.ReadDir("/home"); err == nil {
				for _, entry := range entries {
					if entry.IsDir() && entry.Name() != "lost+found" {
						getRealHomeVal = filepath.Join("/home", entry.Name())
						return
					}
				}
			}
		}
		// Default to standard lookup
		h, _ := os.UserHomeDir()
		if h == "" || h == "/root" {
			// Final fallback: check for any /home/xxx
			if entries, err := os.ReadDir("/home"); err == nil && len(entries) > 0 {
				for _, entry := range entries {
					if entry.IsDir() && entry.Name() != "lost+found" {
						getRealHomeVal = filepath.Join("/home", entry.Name())
						return
					}
				}
			}
		}
		getRealHomeVal = h
	})
	return getRealHomeVal
}

func getShellConfigPath() string {
	home := getRealHomeDir()
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

func resolveBackendPort() int {
	if raw := strings.TrimSpace(os.Getenv("AGENT_BACKEND_PORT")); raw != "" {
		if port, err := strconv.Atoi(raw); err == nil && port > 0 {
			return port
		}
	}

	candidates := []string{".port"}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(sourceFile), ".port"))
	}

	for _, candidate := range candidates {
		b, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		if port, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && port > 0 {
			return port
		}
	}

	return 8080
}

func resolveHookCallbackURL() string {
	if raw := strings.TrimSpace(os.Getenv("AGENT_HOOK_ENDPOINT")); raw != "" {
		return raw
	}
	return fmt.Sprintf("http://127.0.0.1:%d/hooks/event", resolveBackendPort())
}

func isTextLikeMime(mimeType string) bool {
	if mimeType == "" {
		return false
	}
	mt := strings.ToLower(mimeType)
	return strings.HasPrefix(mt, "text/") ||
		strings.Contains(mt, "json") ||
		strings.Contains(mt, "xml") ||
		strings.Contains(mt, "javascript") ||
		strings.Contains(mt, "typescript") ||
		strings.Contains(mt, "markdown") ||
		strings.Contains(mt, "yaml") ||
		strings.Contains(mt, "toml") ||
		strings.Contains(mt, "x-sh") ||
		strings.Contains(mt, "x-c") ||
		strings.Contains(mt, "x-cpp") ||
		strings.Contains(mt, "x-python") ||
		strings.Contains(mt, "x-go") ||
		strings.Contains(mt, "x-rust") ||
		strings.Contains(mt, "x-java")
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	switch ext {
	case "cpp", "cc", "cxx", "hpp":
		return "cpp"
	case "c", "h":
		return "c"
	case "py":
		return "python"
	case "js", "mjs":
		return "javascript"
	case "ts", "mts":
		return "typescript"
	case "go":
		return "go"
	case "rs":
		return "rust"
	case "md":
		return "markdown"
	case "sh", "bash":
		return "bash"
	case "yml", "yaml":
		return "yaml"
	case "json":
		return "json"
	case "html":
		return "html"
	case "css":
		return "css"
	case "sql":
		return "sql"
	case "java":
		return "java"
	default:
		return ext
	}
}

func buildFilePreview(path string) (*FilePreviewResponse, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	res := &FilePreviewResponse{
		Path:      absPath,
		Name:      info.Name(),
		ParentDir: filepath.Dir(absPath),
		IsDir:     info.IsDir(),
		Size:      info.Size(),
		Mode:      info.Mode().String(),
		ModTime:   info.ModTime(),
	}
	if absPath == "/" {
		res.ParentDir = "/"
	}

	if info.IsDir() {
		res.PreviewType = "directory"
		return res, nil
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	head := make([]byte, 512)
	n, readErr := file.Read(head)
	if readErr != nil && readErr != io.EOF {
		return nil, readErr
	}
	head = head[:n]

	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(absPath)))
	if mimeType == "" && len(head) > 0 {
		mimeType = http.DetectContentType(head)
	}
	// Explicit correction for webm which is often misidentified as audio/webm
	if strings.ToLower(filepath.Ext(absPath)) == ".webm" && strings.HasPrefix(mimeType, "audio/") {
		mimeType = "video/webm"
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	res.MimeType = mimeType

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if strings.HasPrefix(mimeType, "image/") {
		res.PreviewType = "image"
		if info.Size() > imagePreviewLimitBytes {
			res.Content = fmt.Sprintf("Image is too large to preview inline (limit: %d MiB).", imagePreviewLimitBytes/(1024*1024))
			res.Truncated = true
			return res, nil
		}

		data, err := io.ReadAll(io.LimitReader(file, imagePreviewLimitBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > imagePreviewLimitBytes {
			data = data[:imagePreviewLimitBytes]
			res.Truncated = true
		}
		res.DataURL = fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
		return res, nil
	}

	if strings.HasPrefix(mimeType, "video/") || strings.HasPrefix(mimeType, "audio/") {
		res.PreviewType = "video" // We'll use 'video' as a generic media type for now
		return res, nil
	}

	previewLimit := int64(binaryPreviewLimitBytes)
	if isTextLikeMime(mimeType) {
		previewLimit = textPreviewLimitBytes
	}

	data, err := io.ReadAll(io.LimitReader(file, previewLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > previewLimit {
		data = data[:previewLimit]
		res.Truncated = true
	}
	if info.Size() > int64(len(data)) {
		res.Truncated = true
	}

	if isTextLikeMime(mimeType) || utf8.Valid(data) {
		res.PreviewType = "text"
		res.Language = detectLanguage(absPath)
		res.Content = string(data)
		return res, nil
	}

	res.PreviewType = "binary"
	res.Content = hex.Dump(data)
	return res, nil
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Bypass auth in debug/test modes or if explicitly disabled
		if gin.Mode() != gin.ReleaseMode || os.Getenv("DISABLE_AUTH") == "true" {
			c.Next()
			return
		}
		if clusterRequestAuthAllowed(c) {
			c.Next()
			return
		}
		token := requestAuthToken(c)
		expectedKey := runtimeSettingsStore.ExpectedToken()
		if token == "" || token != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}

func requestAuthToken(c *gin.Context) string {
	if token := strings.TrimSpace(c.Query("key")); token != "" {
		return token
	}
	if token := strings.TrimSpace(c.GetHeader("X-API-KEY")); token != "" {
		return token
	}
	if authHeader := strings.TrimSpace(c.GetHeader("Authorization")); authHeader != "" {
		lower := strings.ToLower(authHeader)
		if strings.HasPrefix(lower, "bearer ") {
			return strings.TrimSpace(authHeader[len("Bearer "):])
		}
	}
	return ""
}

func buildRuntimeConfigResponseFromSettings(settings RuntimeSettings) RuntimeConfigResponse {
	logPath := strings.TrimSpace(settings.LogFilePath)
	logAlive := false
	if settings.LogPersistenceEnabled && logPath != "" {
		if info, err := os.Stat(logPath); err == nil && !info.IsDir() {
			logAlive = true
		}
	}
	return RuntimeConfigResponse{
		Runtime:                settings,
		MCPEndpoint:            fmt.Sprintf("http://127.0.0.1:%d/mcp", resolveBackendPort()),
		AuthHeaderName:         "X-API-KEY",
		BearerAuthHeaderName:   "Authorization: Bearer",
		PersistedEventLogPath:  logPath,
		PersistedEventLogAlive: logAlive,
	}
}

func buildRuntimeConfigResponse() RuntimeConfigResponse {
	return buildRuntimeConfigResponseFromSettings(runtimeSettingsStore.Snapshot())
}

func getTagName(id uint32) string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	if name, ok := tagMap[id]; ok {
		return name
	}
	return fmt.Sprintf("Tag-%d", id)
}

func getTagID(name string) uint32 {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	if id, ok := tagNameToID[name]; ok {
		return id
	}
	id := nextTagID
	tagMap[id] = name
	tagNameToID[name] = id
	nextTagID++
	return id
}

func deltaUint64(current, previous uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	return 0
}

func resolveWrapperPath() string {
	if override := os.Getenv("AGENT_WRAPPER_PATH"); override != "" {
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			return override
		}
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		sourceDir := filepath.Dir(sourceFile)
		candidates := []string{
			filepath.Join(sourceDir, "..", "agent-wrapper"),
			filepath.Join(sourceDir, "agent-wrapper"),
			filepath.Join(sourceDir, "..", "..", "agent-wrapper"),
		}
		for _, cnd := range candidates {
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for _, rel := range []string{
			"agent-wrapper",
			"../agent-wrapper",
			"../../agent-wrapper",
			"../../../agent-wrapper",
		} {
			cnd := filepath.Clean(filepath.Join(cwd, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		for _, rel := range []string{
			"agent-wrapper",
			"../agent-wrapper",
			"../../agent-wrapper",
		} {
			cnd := filepath.Clean(filepath.Join(execDir, rel))
			if info, err := os.Stat(cnd); err == nil && !info.IsDir() {
				return cnd
			}
		}
	}

	return ""
}

func resolveShellCandidate(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}

	if strings.ContainsRune(candidate, os.PathSeparator) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate
		}
		return ""
	}

	if resolved, err := exec.LookPath(candidate); err == nil {
		return resolved
	}

	for _, prefix := range []string{"/bin/", "/usr/bin/", "/usr/local/bin/"} {
		path := prefix + candidate
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return path
		}
	}

	return ""
}

func resolveShellPath(requested string) string {
	requested = strings.TrimSpace(requested)

	switch strings.ToLower(requested) {
	case "", "auto":
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if resolved := resolveShellCandidate(candidate); resolved != "" {
				return resolved
			}
		}
		return ""
	case "system", "env":
		if resolved := resolveShellCandidate(os.Getenv("SHELL")); resolved != "" {
			return resolved
		}
		for _, candidate := range []string{"fish", "zsh", "bash", "ash", "sh"} {
			if resolved := resolveShellCandidate(candidate); resolved != "" {
				return resolved
			}
		}
		return ""
	default:
		return resolveShellCandidate(requested)
	}
}

func resolveShellWorkDir() string {
	if override := os.Getenv("AGENT_SHELL_DIR"); override != "" {
		if info, err := os.Stat(override); err == nil && info.IsDir() {
			return override
		}
	}

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		repoRoot := filepath.Dir(filepath.Dir(sourceFile))
		if info, err := os.Stat(repoRoot); err == nil && info.IsDir() {
			return repoRoot
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}

	if home, err := os.UserHomeDir(); err == nil {
		return home
	}

	return "/"
}

func setEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	replaced := false
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			replaced = true
			break
		}
	}
	if !replaced {
		env = append(env, prefix+value)
	}
	return env
}

func writePortFile(actualPort int) {
	data := []byte(fmt.Sprintf("%d", actualPort))
	_ = os.WriteFile(".port", data, 0644)

	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		backendDir := filepath.Dir(sourceFile)
		_ = os.WriteFile(filepath.Join(backendDir, ".port"), data, 0644)
	}
}

func getZramStats() (used, total uint64) {
	zramDevices, _ := filepath.Glob("/sys/block/zram*")
	for _, dev := range zramDevices {
		// disksize is the total uncompressed swap capacity of this zram device
		if data, err := os.ReadFile(filepath.Join(dev, "disksize")); err == nil {
			val := strings.TrimSpace(string(data))
			if sz, err := strconv.ParseUint(val, 10, 64); err == nil {
				total += sz
			}
		}
		// mm_stat provides detailed memory usage: orig_data_size compr_data_size mem_used_total ...
		if data, err := os.ReadFile(filepath.Join(dev, "mm_stat")); err == nil {
			var memUsed uint64
			fields := strings.Fields(string(data))
			if len(fields) >= 3 {
				memUsed, _ = strconv.ParseUint(fields[2], 10, 64)
				// used is the actual physical memory consumed by zram (mem_used_total)
				used += memUsed
			}
		} else {
			// fallback to compr_data_size (compressed size) if mm_stat is not available
			if data, err := os.ReadFile(filepath.Join(dev, "compr_data_size")); err == nil {
				var c uint64
				fmt.Sscanf(string(data), "%d", &c)
				used += c
			}
		}
	}
	return
}

func refreshHooksPaths() {
	home := getRealHomeDir()
	log.Printf("[DEBUG] Resolving agent config paths for home: %s", home)
	for i := range availableHooks {
		if availableHooks[i].HookType == HookTypeNative {
			switch availableHooks[i].ID {
			case "claude":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".claude", "settings.json")
			case "gemini":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".gemini", "settings.json")
			case "codex":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".codex", "hooks.json")
				availableHooks[i].NativeFeatureConfigPath = filepath.Join(home, ".codex", "config.toml")
			case "kiro":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".kiro", "agents", "agent-ebpf-hook.json")
			case "copilot":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".copilot", "config.json")
			}
		}
	}
}
