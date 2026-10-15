package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodingHarnessCatalogUniqueAndNoShellInterpolation(t *testing.T) {
	seen := make(map[string]bool)
	for _, h := range codingHarnessCatalog {
		if seen[h.ID] {
			t.Fatalf("duplicate harness %q", h.ID)
		}
		seen[h.ID] = true
		if h.Label == "" || h.Command == "" || strings.ContainsAny(h.Command, "/\\ ;&|$") {
			t.Errorf("invalid executable for %q: %q", h.ID, h.Command)
		}
		if _, err := resolveCodingHarness(h, func(name string) (string, error) {
			if name != h.Command {
				t.Fatalf("unexpected PATH lookup: %q", name)
			}
			return "/opt/bin/" + name, nil
		}); err != nil {
			t.Errorf("unexpected resolution failure: %v", err)
		}
	}
}

func TestCodingHarnessCommandExactArgsAndMissingProgram(t *testing.T) {
	spec := codingHarnessSpec{ID: "kiro", Label: "Kiro CLI", Command: "kiro-cli", Args: []string{"chat"}}
	got, err := resolveCodingHarness(spec, func(string) (string, error) {
		return "/usr/local/bin/kiro-cli", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/local/bin/kiro-cli", "chat"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command = %q, want %q", got, want)
	}
	_, err = resolveCodingHarness(spec, func(string) (string, error) { return "", os.ErrNotExist })
	if !errors.Is(err, os.ErrNotExist) && (err == nil || !strings.Contains(err.Error(), "PATH")) {
		t.Fatalf("missing executable should yield an actionable error: %v", err)
	}
}

func TestDeepSeekCommandAndLocalURL(t *testing.T) {
	const port = 39991
	if got := deepSeekURL(port); got != "http://localhost:39991/" {
		t.Fatalf("local web URL = %q", got)
	}
	plain := deepSeekCommand("/usr/bin/dsh", false, port)
	if !reflect.DeepEqual(plain, []string{"/usr/bin/dsh", "web", "--no-open", "--host", "127.0.0.1", "--port", "39991"}) {
		t.Fatalf("unexpected direct command: %q", plain)
	}
	npx := deepSeekCommand("/usr/bin/npx", true, port)
	if !reflect.DeepEqual(npx, []string{"/usr/bin/npx", "--yes", "@deepseek-ai/dsh", "web", "--no-open", "--host", "127.0.0.1", "--port", "39991"}) {
		t.Fatalf("unexpected npx command: %q", npx)
	}
}

func TestDeepSeekPortSelectionUsesEphemeralLoopback(t *testing.T) {
	p, err := newDeepSeekPort()
	if err != nil {
		t.Skipf("loopback sockets unavailable: %v", err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("invalid port: %d", p)
	}
	if got := deepSeekURL(p); got != fmt.Sprintf("http://localhost:%d/", p) {
		t.Fatal("the UI URL must remain loopback-only")
	}
}

func TestCodingHarnessDoesNotAcceptExecutablePathFromCatalog(t *testing.T) {
	for _, bad := range []string{"../sh", "cmd/evil", "foo\\bar"} {
		_, err := resolveCodingHarness(codingHarnessSpec{ID: "bad", Command: bad}, func(s string) (string, error) {
			return filepath.Join("/tmp", s), nil
		})
		if err == nil {
			t.Errorf("accepted executable path %q", bad)
		}
	}
}
