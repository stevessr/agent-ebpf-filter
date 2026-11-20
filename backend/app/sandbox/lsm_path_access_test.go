package sandbox

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestFileAccessPathValidation(t *testing.T) {
	for _, bad := range []string{"relative", "", "/", "/tmp/..", string([]byte{'/', 'x', 0, 'y'})} {
		if _, err := NormalizeFileAccessPath(bad); err == nil {
			t.Errorf("accepted invalid file path %q", bad)
		}
	}
	dir := t.TempDir()
	if _, err := NormalizeFileAccessPath(dir); err == nil {
		t.Fatal("directory was accepted as an exact file")
	}
	file := filepath.Join(dir, "token")
	if err := os.WriteFile(file, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := NormalizeFileAccessPath(file); err != nil || got != file {
		t.Fatalf("exact file: %q, %v", got, err)
	}
}
func TestFileAccessRequiresCanonicalRegularExistingTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "safe-key")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateProtectedFile(file); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{dir, filepath.Join(dir, "missing")} {
		if err := validateProtectedFile(target); err == nil {
			t.Errorf("accepted unsafe path %q", target)
		}
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(file, alias); err != nil {
		t.Fatal(err)
	}
	if err := validateProtectedFile(alias); err == nil {
		t.Fatal("accepted symlinked target")
	}
	linkedDir := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(dir, linkedDir); err != nil {
		t.Fatal(err)
	}
	if err := validateProtectedFile(filepath.Join(linkedDir, "safe-key")); err == nil {
		t.Fatal("accepted symlinked parent directory")
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

func TestLsmInodeKeyABIAndHardlinkIdentity(t *testing.T) {
	if got := unsafe.Sizeof(LsmInodeKey{}); got != 16 {
		t.Fatalf("LsmInodeKey size = %d, want 16", got)
	}

	// Standard (major=8, minor=1) -> glibc makedev = 0x0801, kernel MKDEV = (8<<20)|1
	if got := KernelDevFromStatDev(0x0801); got != (8<<20)|1 {
		t.Fatalf("KernelDevFromStatDev(0x0801) = %#x, want %#x", got, (8<<20)|1)
	}
	// Extended (major=259, minor=300): minor high bits live in bits 12..19 -> (300&0xff)|((300&0xfff00)<<12)
	extDev := uint64((259&0xfff)<<8) | uint64(300&0xff) | (uint64(300 & ^0xff) << 12)
	if got := KernelDevFromStatDev(extDev); got != (259<<20)|300 {
		t.Fatalf("KernelDevFromStatDev(ext 259,300) = %#x, want %#x", got, (259<<20)|300)
	}

	dir := t.TempDir()
	orig := filepath.Join(dir, "secret.env")
	hardlink := filepath.Join(dir, "innocent.txt")
	if err := os.WriteFile(orig, []byte("SECRET=1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(orig, hardlink); err != nil {
		t.Fatal(err)
	}
	k1, err := InodeKeyForPath(orig)
	if err != nil {
		t.Fatalf("InodeKeyForPath(orig): %v", err)
	}
	k2, err := InodeKeyForPath(hardlink)
	if err != nil {
		t.Fatalf("InodeKeyForPath(hardlink): %v", err)
	}
	if k1 != k2 || k1.Ino == 0 {
		t.Fatalf("hardlink inode keys differ: orig=%+v link=%+v", k1, k2)
	}
}
