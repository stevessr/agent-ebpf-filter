package main

import "testing"

func TestCriticalSystemReadProtectionRequiresTypedConfirmation(t *testing.T) {
	for _, path := range []string{"/etc/shadow", "/etc/passwd", "/etc/sudoers", "/etc/gshadow"} {
		if !needsDangerousReadConfirmation(fileAccessRule{Path: path, DenyRead: true}) {
			t.Fatalf("%s read protection must require elevated confirmation", path)
		}
		if needsDangerousReadConfirmation(fileAccessRule{Path: path, DenyWrite: true}) {
			t.Fatalf("%s write-only protection should retain ordinary confirmation", path)
		}
	}
	if needsDangerousReadConfirmation(fileAccessRule{Path: "/tmp/test", DenyRead: true}) {
		t.Fatal("ordinary test file should not demand elevated confirmation")
	}
}
