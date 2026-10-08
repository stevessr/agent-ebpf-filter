package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileAccessPathValidation(t *testing.T) {
	for _, bad := range []string{"relative", "", "/", "/tmp/..", string([]byte{'/', 'x', 0, 'y'})} {
		if _, err := NormalizeFileAccessPath(bad); err == nil { t.Errorf("accepted invalid file path %q", bad) }
	}
	dir := t.TempDir()
	if _, err := NormalizeFileAccessPath(dir); err == nil { t.Fatal("directory was accepted as an exact file") }
	file := filepath.Join(dir, "token")
	if err := os.WriteFile(file, []byte("test"), 0600); err != nil { t.Fatal(err) }
	if got, err := NormalizeFileAccessPath(file); err != nil || got != file {
		t.Fatalf("exact file: %q, %v", got, err)
	}
}
func TestPathPolicyBitsPreserveLegacyExec(t *testing.T) {
	if got := mergedPathBits(denyExec, denyRead, true); got != denyExec|denyRead {
		t.Fatalf("add read bit destroyed exec bit: %b", got)
	}
	if got := mergedPathBits(denyExec|denyRead|denyWrite, denyRead|denyWrite, false); got != denyExec {
		t.Fatalf("revoke file access destroyed legacy exec deny: %b", got)
	}
	if got := mergedPathBits(denyRead, denyExec, false); got != denyRead {
		t.Fatalf("unblock exec destroyed read policy: %b", got)
	}
}
