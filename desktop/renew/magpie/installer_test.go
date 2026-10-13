package magpie

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPlanUsesYetoneMagpieNotApacheMarketplace(t *testing.T) {
	app, err := Plan(InstallApp, "")
	if err != nil {
		t.Fatal(err)
	}
	if app.Display() != "go install github.com/yetone/magpie@latest" || app.Timeout < 5*time.Minute {
		t.Fatalf("unexpected application plan: %+v", app)
	}
	plugin, err := Plan(AddPlugin, "@magpie-community/middleware-model-map")
	if err != nil {
		t.Fatal(err)
	}
	if plugin.Display() != "magpie plugin add @magpie-community/middleware-model-map" {
		t.Fatalf("unexpected plugin command: %s", plugin.Display())
	}
	updates, err := Plan(UpdatePlugins, "")
	if err != nil || updates.Display() != "magpie plugin update" {
		t.Fatalf("unexpected plugin update: %+v, %v", updates, err)
	}
}

func TestValidPackageOnlyNpmSpecs(t *testing.T) {
	good := []string{
		"opencode-gemini-auth", "pi-antigravity",
		"@magpie-community/middleware-model-map",
		"@magpie-community/middleware-model-map@1.2.3",
	}
	for _, name := range good {
		if !ValidPackage(name) {
			t.Errorf("rejected npm package %q", name)
		}
	}
	bad := []string{
		"", "../plugin.js", "./local", "/tmp/plugin.js",
		"git+https://github.com/attacker/plugin",
		"github:owner/repo", "--help", "-v", "abc def",
		"foo;echo", "foo\nbar", "foo\\bar",
		"@bad", "@scope/", "@scope/../bad",
		strings.Repeat("a", 161),
	}
	for _, name := range bad {
		if ValidPackage(name) {
			t.Errorf("accepted unsafe package %q", name)
		}
		if _, err := Plan(AddPlugin, name); err == nil {
			t.Errorf("planned unsafe package %q", name)
		}
	}
	if _, err := Plan("delete-all", ""); err == nil {
		t.Fatal("arbitrary action was accepted")
	}
}

func TestBoundedOutputDoesNotAccumulateSecrets(t *testing.T) {
	var b boundedOutput
	input := []byte(strings.Repeat("s", 25000))
	n, err := b.Write(input)
	if n != len(input) || err != nil || len(b.buffer) != 16<<10 {
		t.Fatalf("write n=%d err=%v buffered=%d", n, err, len(b.buffer))
	}
	n, _ = b.Write(input)
	if n != len(input) || len(b.buffer) != 16<<10 {
		t.Fatal("output buffer exceeded bound")
	}
}

func TestPlansDoNotShareMutableArgv(t *testing.T) {
	a, _ := Plan(AddPlugin, "pi-antigravity")
	b, _ := Plan(AddPlugin, "pi-antigravity")
	a.Args[2] = "modified"
	if reflect.DeepEqual(a.Args, b.Args) {
		t.Fatal("plans must not share argv")
	}
}

type recorder struct { called bool; step Step }
func (r *recorder) Run(_ context.Context, step Step) error {
	r.called, r.step = true, step
	return nil
}

func TestExecuteRejectsUnknownCommandEvenAsRoot(t *testing.T) {
	r := &recorder{}
	err := Execute(context.Background(), r, "shell", "echo")
	if err == nil || r.called {
		t.Fatalf("unsafe command passed execution boundary: %v", err)
	}
}

func TestNoShellInPlans(t *testing.T) {
	for _, action := range []string{InstallApp, AddPlugin, UpdatePlugins} {
		step, err := Plan(action, "pi-antigravity")
		if err != nil {
			t.Fatal(err)
		}
		if step.Program != "go" && step.Program != "magpie" {
			t.Fatalf("unexpected executable %q", step.Program)
		}
	}
}
