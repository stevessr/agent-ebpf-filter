package handlers

import (
	"debug/elf"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCappedBufferBoundsOutput(t *testing.T) {
	w := &cappedBuffer{remaining: 4}
	if n, err := w.Write([]byte("abcdef")); err != nil || n != 6 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if got := w.buf.String(); got != "abcd" || !w.truncated {
		t.Fatalf("buffer = %q, truncated = %t", got, w.truncated)
	}
}

func TestELFMetadataWithinLimitRejectsOversizedSymbolData(t *testing.T) {
	f := &elf.File{Sections: []*elf.Section{
		{SectionHeader: elf.SectionHeader{Type: elf.SHT_SYMTAB, Size: maxELFMetadataBytes + 1}},
	}}
	if elfMetadataWithinLimit(f) {
		t.Fatal("oversized symbol metadata accepted")
	}
}

func TestHandleFileELFRejectsOversizedFileBeforeParsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := filepath.Join(t.TempDir(), "large.elf")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxELFPreviewFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/system/file-elf?path="+url.QueryEscape(path), nil)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	HandleFileELF(context)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "size limit") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}
