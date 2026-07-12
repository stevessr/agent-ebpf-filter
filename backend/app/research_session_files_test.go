package app

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestResearchFilesRejectUnsafeTargets(t *testing.T) {
	base := t.TempDir()
	_, session, err := openResearchSession(base, "rs_safe", true)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	outside := filepath.Join(t.TempDir(), "outside")
	if err = os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		setup func(string) error
	}{
		{"symlink.json", func(p string) error { return os.Symlink(outside, p) }},
		{"hardlink.json", func(p string) error { return os.Link(outside, p) }},
		{"fifo.json", func(p string) error { return syscall.Mkfifo(p, 0600) }},
	}
	for _, tc := range cases {
		p := filepath.Join(base, "rs_safe", tc.name)
		if err = tc.setup(p); err != nil {
			t.Fatal(err)
		}
		if err = atomicWriteResearchFile(session, tc.name, []byte("changed"), researchSessionFileMaxBytes); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
		if _, err = readResearchFile(session, tc.name, researchSessionFileMaxBytes); err == nil {
			t.Fatalf("%s read", tc.name)
		}
		if err = removeResearchFile(session, tc.name); err == nil {
			t.Fatalf("%s removed", tc.name)
		}
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "unchanged" {
		t.Fatalf("outside changed: %q %v", got, err)
	}
}
func TestResearchFilesAtomicPrivateRoundTrip(t *testing.T) {
	base := t.TempDir()
	root, session, err := openResearchSession(base, "rs_safe", true)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	defer session.Close()
	if err = atomicWriteResearchFile(session, "session.json", []byte("first"), researchSessionFileMaxBytes); err != nil {
		t.Fatal(err)
	}
	if err = atomicWriteResearchFile(session, "session.json", []byte("second"), researchSessionFileMaxBytes); err != nil {
		t.Fatal(err)
	}
	got, err := readResearchFile(session, "session.json", researchSessionFileMaxBytes)
	if err != nil || string(got) != "second" {
		t.Fatalf("got %q %v", got, err)
	}
	info, err := os.Stat(filepath.Join(base, "rs_safe", "session.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v %v", info.Mode(), err)
	}
}
func TestResearchFilesRejectSymlinkedSession(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(base, "rs_safe")); err != nil {
		t.Fatal(err)
	}
	if root, session, err := openResearchSession(base, "rs_safe", true); err == nil {
		root.Close()
		session.Close()
		t.Fatal("symlink session accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "session.json")); !os.IsNotExist(err) {
		t.Fatalf("outside touched: %v", err)
	}
}
