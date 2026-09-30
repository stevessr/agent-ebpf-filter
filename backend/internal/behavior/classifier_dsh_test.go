package behavior

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestClassifyBehaviorRecognizesDshPluginOperation(t *testing.T) {
	classification := ClassifyBehavior("dsh", []string{"plugin", "--profile", "tui", "add", "@scope/pkg"})
	if classification.PrimaryCategory != "PACKAGE_MANAGER" {
		t.Fatalf("primary category = %q", classification.PrimaryCategory)
	}
	if classification.Confidence != "high" {
		t.Fatalf("confidence = %q", classification.Confidence)
	}
}

func TestClassifyBehaviorFlagsDshVersionExemptionAsSensitive(t *testing.T) {
	classification := ClassifyBehavior("dsh", []string{
		"plugin", "--profile", "tui", "allow-version", "pkg@1.0.0",
		"--dsh-version", "0.2.0", "--accept-risk",
	})
	if classification.PrimaryCategory != "SENSITIVE" {
		t.Fatalf("primary category = %q", classification.PrimaryCategory)
	}
	var hasPackageManager, hasSensitive bool
	for _, category := range classification.Categories {
		hasPackageManager = hasPackageManager || category == pb.BehaviorCategory_PACKAGE_MANAGER
		hasSensitive = hasSensitive || category == pb.BehaviorCategory_SENSITIVE
	}
	if !hasPackageManager || !hasSensitive {
		t.Fatalf("categories = %#v", classification.Categories)
	}
}

func TestClassifyBehaviorRecognizesDshConfigDumpWithoutInspectingAppArgs(t *testing.T) {
	dump := ClassifyBehavior("dsh", []string{"web", "--dump-config"})
	if dump.PrimaryCategory != "SYSTEM_INFO" {
		t.Fatalf("dump primary category = %q", dump.PrimaryCategory)
	}

	appArgs := ClassifyBehavior("dsh", []string{"headless", "prompt", "--dump-config"})
	if appArgs.PrimaryCategory != "UNKNOWN" {
		t.Fatalf("app suffix must remain opaque, got %q", appArgs.PrimaryCategory)
	}
}
