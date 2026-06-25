package app

import (
	"agent-ebpf-filter/app/platform"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ---- moved from backend/zz_merged_backend.go section helpers_fs.go ----

const hookMarker = "agent-ebpf-hook-active"
const kiroManagedAgent = "agent-ebpf-hook"
const (
	textPreviewLimitBytes   = 64 * 1024
	binaryPreviewLimitBytes = 4 * 1024
	imagePreviewLimitBytes  = 2 * 1024 * 1024
)

var (
)

// writeFileAsRealUser writes a file with the real user's ownership instead of root

// mkdirAllAsRealUser creates directories with the real user's ownership


func getShellConfigPath() string {
	home := platform.GetRealHomeDir()
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
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

func detectTextEncoding(data []byte) string {
	if len(data) >= 2 {
		if data[0] == 0xff && data[1] == 0xfe {
			return "utf-16le"
		}
		if data[0] == 0xfe && data[1] == 0xff {
			return "utf-16be"
		}
	}
	if len(data) >= 16 {
		evenNUL := 0
		oddNUL := 0
		for i, b := range data {
			if b != 0 {
				continue
			}
			if i%2 == 0 {
				evenNUL++
			} else {
				oddNUL++
			}
		}
		pairs := len(data) / 2
		if pairs > 0 && oddNUL*2 > pairs {
			return "utf-16le"
		}
		if pairs > 0 && evenNUL*2 > pairs {
			return "utf-16be"
		}
	}
	return "utf-8"
}

func decodeTextPreview(data []byte, encoding string) string {
	if encoding != "utf-16le" && encoding != "utf-16be" {
		return string(data)
	}
	start := 0
	if len(data) >= 2 && ((data[0] == 0xff && data[1] == 0xfe) || (data[0] == 0xfe && data[1] == 0xff)) {
		start = 2
	}
	if (len(data)-start)%2 != 0 {
		data = data[:len(data)-1]
	}
	units := make([]uint16, 0, (len(data)-start)/2)
	for i := start; i+1 < len(data); i += 2 {
		if encoding == "utf-16le" {
			units = append(units, uint16(data[i])|uint16(data[i+1])<<8)
		} else {
			units = append(units, uint16(data[i])<<8|uint16(data[i+1]))
		}
	}
	return string(utf16.Decode(units))
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
	res.Hexable = true

	if len(head) >= 4 && head[0] == 0x7f && head[1] == 'E' && head[2] == 'L' && head[3] == 'F' {
		res.PreviewType = "elf"
		res.MimeType = "application/x-elf"
		return res, nil
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if mimeType == "application/pdf" || strings.ToLower(filepath.Ext(absPath)) == ".pdf" {
		res.PreviewType = "pdf"
		res.MimeType = "application/pdf"
		return res, nil
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

	encoding := detectTextEncoding(data)
	if isTextLikeMime(mimeType) || utf8.Valid(data) || encoding != "utf-8" {
		res.PreviewType = "text"
		res.Language = detectLanguage(absPath)
		res.Encoding = encoding
		res.Content = decodeTextPreview(data, encoding)
		res.Streamable = res.Truncated
		return res, nil
	}

	res.PreviewType = "binary"
	res.Content = hex.Dump(data)
	return res, nil
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
	home := platform.GetRealHomeDir()
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
			case "augment":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".augment", "settings.json")
			case "antigravity":
				availableHooks[i].NativeConfigPath = filepath.Join(home, ".gemini", "antigravity-cli", "plugins", hookMarker, "hooks.json")
			}
		}
	}
}
