package handlers

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
)

const (
	maxUploadedFileBytes   = int64(64 << 20)
	maxUploadRequestBytes  = maxUploadedFileBytes + (1 << 20)
	maxELFPreviewFileBytes = int64(256 << 20)
	maxELFMetadataBytes    = uint64(8 << 20)
	maxELFSymbolEntries    = uint64(64 << 10)
	maxELFCommandBytes     = 2 << 20
)

type cappedBuffer struct {
	buf       bytes.Buffer
	remaining int
	truncated bool
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > w.remaining {
		w.truncated = true
	}
	if w.remaining > 0 {
		keep := len(p)
		if keep > w.remaining {
			keep = w.remaining
		}
		_, _ = w.buf.Write(p[:keep])
		w.remaining -= keep
	}
	return n, nil
}

func elfMetadataWithinLimit(f *elf.File) bool {
	var total uint64
	var symbolEntries uint64
	seen := make(map[int]struct{})
	for i, section := range f.Sections {
		if section.Type != elf.SHT_SYMTAB && section.Type != elf.SHT_DYNSYM && section.Type != elf.SHT_DYNAMIC {
			continue
		}
		if section.Type == elf.SHT_SYMTAB || section.Type == elf.SHT_DYNSYM {
			if section.Entsize == 0 && section.Size > 0 {
				return false
			}
			if section.Entsize > 0 {
				entries := section.Size / section.Entsize
				if entries > maxELFSymbolEntries || symbolEntries > maxELFSymbolEntries-entries {
					return false
				}
				symbolEntries += entries
			}
		}
		for _, index := range []int{i, int(section.Link)} {
			if index < 0 || index >= len(f.Sections) {
				continue
			}
			if _, ok := seen[index]; ok {
				continue
			}
			seen[index] = struct{}{}
			size := f.Sections[index].Size
			if size > maxELFMetadataBytes || total > maxELFMetadataBytes-size {
				return false
			}
			total += size
		}
	}
	return true
}

var (
	errInvalidUploadFilename = errors.New("invalid upload filename")
	errUploadedFileTooLarge  = errors.New("uploaded file exceeds size limit")
	errUploadDestinationUsed = errors.New("upload destination already exists")
)

func HandleSystemLs(c *gin.Context) {
	p := c.DefaultQuery("path", "/")
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "0"))
	showHidden := c.Query("showHidden") == "true"

	e, err := os.ReadDir(p)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	var filtered []os.DirEntry
	for _, v := range e {
		if !showHidden && strings.HasPrefix(v.Name(), ".") {
			continue
		}
		filtered = append(filtered, v)
	}

	total := len(filtered)
	start := offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := total
	if limit > 0 && start+limit < total {
		end = start + limit
	}

	slice := filtered[start:end]
	l := []gin.H{}
	for _, v := range slice {
		fp := filepath.Join(p, v.Name())
		mType := ""
		var size int64
		var modTime string
		info, err := v.Info()
		if err == nil {
			size = info.Size()
			modTime = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
			if !v.IsDir() {
				mType = mime.TypeByExtension(filepath.Ext(v.Name()))
			}
		}
		l = append(l, gin.H{"name": v.Name(), "isDir": v.IsDir(), "path": fp, "mimeType": mType, "size": size, "modTime": modTime})
	}
	c.JSON(200, gin.H{"items": l, "total": total, "offset": start, "limit": limit})
}

func HandleFilePreview(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	preview, err := Deps.BuildFilePreview(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "path not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, preview)
}

func HandleFilePreviewStream(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}

	previewRaw, err := Deps.BuildFilePreview(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(404, gin.H{"error": "path not found"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	preview, ok := previewRaw.(*FilePreviewResponse)
	if !ok {
		c.JSON(500, gin.H{"error": "invalid preview response"})
		return
	}
	if preview.IsDir || preview.PreviewType != "text" {
		c.JSON(415, gin.H{"error": "only text files can be streamed"})
		return
	}

	file, err := os.Open(preview.Path)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	c.Header("X-File-Size", strconv.FormatInt(preview.Size, 10))
	c.Status(http.StatusOK)

	flusher, _ := c.Writer.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}

		n, readErr := file.Read(buf)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buf[:n]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr == io.EOF {
			return
		}
		if readErr != nil {
			return
		}
	}
}

func printableASCII(b byte) string {
	if b >= 32 && b <= 126 && unicode.IsPrint(rune(b)) {
		return string([]byte{b})
	}
	return "."
}

func HandleFileHex(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "4096"))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 64*1024 {
		limit = 4096
	}

	absPath, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	info, err := os.Stat(absPath)
	if err != nil || !info.Mode().IsRegular() {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	file, err := os.Open(absPath)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()

	buf := make([]byte, limit)
	n, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	buf = buf[:n]
	rows := []gin.H{}
	for i := 0; i < len(buf); i += 16 {
		end := i + 16
		if end > len(buf) {
			end = len(buf)
		}
		chunk := buf[i:end]
		hexCells := make([]string, 0, 16)
		ascii := strings.Builder{}
		for _, b := range chunk {
			hexCells = append(hexCells, fmt.Sprintf("%02x", b))
			ascii.WriteString(printableASCII(b))
		}
		rows = append(rows, gin.H{"offset": offset + int64(i), "hex": hexCells, "ascii": ascii.String()})
	}
	nextOffset := offset + int64(n)
	c.JSON(200, gin.H{
		"path":       absPath,
		"size":       info.Size(),
		"offset":     offset,
		"limit":      limit,
		"bytesRead":  n,
		"nextOffset": nextOffset,
		"eof":        nextOffset >= info.Size(),
		"rows":       rows,
	})
}

func HandleFileELF(c *gin.Context) {
	targetPath := strings.TrimSpace(c.Query("path"))
	if targetPath == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	absPath, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	file, err := os.Open(absPath)
	if err != nil {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	if info.Size() > maxELFPreviewFileBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "ELF file exceeds preview size limit"})
		return
	}
	f, err := elf.NewFile(file)
	if err != nil {
		c.JSON(415, gin.H{"error": "not an ELF file"})
		return
	}
	defer f.Close()

	sections := []gin.H{}
	for i, s := range f.Sections {
		if i >= 128 {
			break
		}
		sections = append(sections, gin.H{"name": s.Name, "type": s.Type.String(), "flags": s.Flags.String(), "addr": s.Addr, "offset": s.Offset, "size": s.Size})
	}
	programs := []gin.H{}
	for i, p := range f.Progs {
		if i >= 64 {
			break
		}
		programs = append(programs, gin.H{"type": p.Type.String(), "flags": p.Flags.String(), "vaddr": p.Vaddr, "off": p.Off, "filesz": p.Filesz, "memsz": p.Memsz})
	}
	var dynlibs []string
	var dynSyms, staticSyms []elf.Symbol
	metadataError := ""
	if elfMetadataWithinLimit(f) {
		dynlibs, _ = f.DynString(elf.DT_NEEDED)
		dynSyms, _ = f.DynamicSymbols()
		staticSyms, _ = f.Symbols()
	} else {
		metadataError = "ELF symbol metadata exceeds preview size limit."
	}
	limitSymbols := func(in []elf.Symbol) []gin.H {
		out := []gin.H{}
		for i, s := range in {
			if i >= 120 {
				break
			}
			out = append(out, gin.H{"name": s.Name, "info": s.Info, "other": s.Other, "section": s.Section.String(), "value": s.Value, "size": s.Size})
		}
		return out
	}

	disassembly := ""
	disassemblyError := ""
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "objdump", "-d", "--demangle", "--no-show-raw-insn", "/proc/self/fd/3")
	cmd.ExtraFiles = []*os.File{file}
	stdout := &cappedBuffer{remaining: maxELFCommandBytes}
	stderr := &cappedBuffer{remaining: maxELFCommandBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err == nil {
		lines := strings.Split(stdout.buf.String(), "\n")
		if len(lines) > 240 {
			lines = lines[:240]
			disassemblyError = "Disassembly preview truncated to first 240 lines."
		}
		if stdout.truncated {
			disassemblyError = "Disassembly output exceeded size limit."
		}
		disassembly = strings.Join(lines, "\n")
	} else {
		disassemblyError = strings.TrimSpace(fmt.Sprintf("%v: %s", err, stderr.buf.String()))
		if stderr.truncated {
			disassemblyError += " (error output truncated)"
		}
	}

	c.JSON(200, gin.H{
		"path": absPath,
		"size": info.Size(),
		"header": gin.H{
			"class": f.Class.String(), "data": f.Data.String(), "version": f.Version.String(), "osabi": f.OSABI.String(),
			"abiVersion": f.ABIVersion, "type": f.Type.String(), "machine": f.Machine.String(), "entry": f.Entry,
		},
		"sections":           sections,
		"programs":           programs,
		"dynamicLibraries":   dynlibs,
		"dynamicSymbols":     limitSymbols(dynSyms),
		"staticSymbols":      limitSymbols(staticSyms),
		"metadataError":      metadataError,
		"disassembly":        disassembly,
		"disassemblyError":   disassemblyError,
		"dynamicSymbolCount": len(dynSyms),
		"staticSymbolCount":  len(staticSyms),
	})
}

func HandleSystemHome(c *gin.Context) {
	c.JSON(200, gin.H{"path": Deps.GetRealHomeDir()})
}

func HandleDownload(c *gin.Context) {
	p := c.Query("path")
	if p == "" {
		c.JSON(400, gin.H{"error": "path is required"})
		return
	}
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		c.JSON(404, gin.H{"error": "file not found"})
		return
	}
	c.File(p)
}

func HandleUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadRequestBytes)
	dir := c.Query("path")
	if dir == "" {
		dir = Deps.GetRealHomeDir()
	}
	file, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": errUploadedFileTooLarge.Error()})
			return
		}
		c.JSON(400, gin.H{"error": "no file uploaded"})
		return
	}
	dst, err := saveUploadedFileWithinRoot(dir, file)
	if err != nil {
		switch {
		case errors.Is(err, errUploadedFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		case errors.Is(err, errInvalidUploadFilename):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, errUploadDestinationUsed):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(500, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(200, gin.H{"status": "ok", "path": dst})
}

func saveUploadedFileWithinRoot(dir string, header *multipart.FileHeader) (string, error) {
	if header == nil {
		return "", errInvalidUploadFilename
	}
	filename, err := sanitizeUploadedFilename(header.Filename)
	if err != nil {
		return "", err
	}
	if header.Size < 0 || header.Size > maxUploadedFileBytes {
		return "", errUploadedFileTooLarge
	}

	absDir, err := filepath.Abs(filepath.Clean(strings.TrimSpace(dir)))
	if err != nil {
		return "", fmt.Errorf("resolve upload directory: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return "", fmt.Errorf("inspect upload directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("upload path is not a directory")
	}

	root, err := os.OpenRoot(absDir)
	if err != nil {
		return "", fmt.Errorf("open upload directory: %w", err)
	}
	defer root.Close()
	src, err := header.Open()
	if err != nil {
		return "", fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()
	dst, err := root.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("%w: %s", errUploadDestinationUsed, filename)
		}
		return "", fmt.Errorf("create upload destination: %w", err)
	}
	removeDestination := true
	defer func() {
		_ = dst.Close()
		if removeDestination {
			_ = root.Remove(filename)
		}
	}()

	written, err := io.Copy(dst, io.LimitReader(src, maxUploadedFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("write uploaded file: %w", err)
	}
	if written > maxUploadedFileBytes {
		return "", errUploadedFileTooLarge
	}
	if err := dst.Sync(); err != nil {
		return "", fmt.Errorf("sync uploaded file: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", fmt.Errorf("close uploaded file: %w", err)
	}
	removeDestination = false
	return filepath.Join(absDir, filename), nil
}

func sanitizeUploadedFilename(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	filename := filepath.Base(raw)
	if filename == "" || filename == "." || filename == string(filepath.Separator) || strings.ContainsRune(filename, 0) {
		return "", errInvalidUploadFilename
	}
	if len([]byte(filename)) > 255 {
		return "", fmt.Errorf("%w: name is too long", errInvalidUploadFilename)
	}
	return filename, nil
}
