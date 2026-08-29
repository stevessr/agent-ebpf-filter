package tls

import "testing"

func TestParseProcMapLibraryLineDeletedTLSLibrary(t *testing.T) {
	line := "7f3a0000-7f3b0000 r-xp 00000000 08:01 12345 /usr/lib/x86_64-linux-gnu/libssl.so.3 (deleted)"
	mappingRange, perms, path, deleted, ok := parseProcMapLibraryLine(line)
	if !ok || !deleted {
		t.Fatalf("parse = range=%q perms=%q path=%q deleted=%v ok=%v", mappingRange, perms, path, deleted, ok)
	}
	if mappingRange != "7f3a0000-7f3b0000" || perms != "r-xp" {
		t.Fatalf("mapping = %q %q", mappingRange, perms)
	}
	if path != "/usr/lib/x86_64-linux-gnu/libssl.so.3" {
		t.Fatalf("path = %q", path)
	}
}

func TestParseProcMapLibraryLinePreservesSpacesInPath(t *testing.T) {
	line := "7f3a0000-7f3b0000 r-xp 00000000 08:01 12345 /opt/My Agent/libssl.so.3 (deleted)"
	_, _, path, deleted, ok := parseProcMapLibraryLine(line)
	if !ok || !deleted {
		t.Fatalf("parse = path=%q deleted=%v ok=%v", path, deleted, ok)
	}
	if path != "/opt/My Agent/libssl.so.3" {
		t.Fatalf("path = %q", path)
	}
}

func TestParseProcMapLibraryLineRejectsNonTLSLibraries(t *testing.T) {
	for _, line := range []string{
		"7f3a0000-7f3b0000 r-xp 00000000 08:01 12345 /usr/lib/libcrypto.so.3",
		"7f3a0000-7f3b0000 r-xp 00000000 00:00 0 [heap]",
		"7f3a0000-7f3b0000 r-xp 00000000 08:01 12345 /usr/lib/libssl_helper.a",
	} {
		if _, _, path, deleted, ok := parseProcMapLibraryLine(line); ok {
			t.Fatalf("unexpected TLS mapping for %q: path=%q deleted=%v", line, path, deleted)
		}
	}
}

func TestTLSLibraryBaseSupportsOrdinaryPath(t *testing.T) {
	if got := tlsLibraryBase("/usr/lib/libssl.so.3"); got != "libssl.so.3" {
		t.Fatalf("tlsLibraryBase = %q", got)
	}
}
