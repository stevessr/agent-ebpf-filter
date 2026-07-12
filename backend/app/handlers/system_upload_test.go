package handlers

import (
	"bytes"
	"errors"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeUploadedFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "report.txt", want: "report.txt"},
		{input: "../escape.txt", want: "escape.txt"},
		{input: `C:\\fakepath\\report.txt`, want: "report.txt"},
		{input: ".", wantErr: true},
		{input: "", wantErr: true},
		{input: strings.Repeat("x", 256), wantErr: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := sanitizeUploadedFilename(tc.input)
			if tc.wantErr {
				if !errors.Is(err, errInvalidUploadFilename) {
					t.Fatalf("sanitizeUploadedFilename(%q) error = %v", tc.input, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("sanitizeUploadedFilename(%q) = %q, %v; want %q", tc.input, got, err, tc.want)
			}
		})
	}
}

func TestSaveUploadedFileWithinRootIsBoundedAndNonOverwriting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	header := testMultipartFileHeader(t, "../safe.txt", []byte("payload"))
	dst, err := saveUploadedFileWithinRoot(dir, header)
	if err != nil {
		t.Fatalf("saveUploadedFileWithinRoot() error = %v", err)
	}
	if dst != filepath.Join(dir, "safe.txt") {
		t.Fatalf("destination = %q", dst)
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "payload" {
		t.Fatalf("saved data = %q, %v", data, err)
	}
	if _, err := saveUploadedFileWithinRoot(dir, header); !errors.Is(err, errUploadDestinationUsed) {
		t.Fatalf("second upload error = %v, want destination conflict", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "safe.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("upload escaped the selected directory")
	}
}

func TestSaveUploadedFileWithinRootRejectsOversizedHeader(t *testing.T) {
	t.Parallel()
	header := testMultipartFileHeader(t, "large.bin", []byte("small"))
	header.Size = maxUploadedFileBytes + 1
	if _, err := saveUploadedFileWithinRoot(t.TempDir(), header); !errors.Is(err, errUploadedFileTooLarge) {
		t.Fatalf("oversized upload error = %v", err)
	}
}

func testMultipartFileHeader(t *testing.T, filename string, payload []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), writer.Boundary())
	form, err := reader.ReadForm(int64(len(payload) + 1024))
	if err != nil {
		t.Fatalf("ReadForm() error = %v", err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })
	return form.File["file"][0]
}
