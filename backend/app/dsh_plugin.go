package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"agent-ebpf-filter/app/platform"
)

const dshSubprocessPluginPackage = "@agent-ebpf/dsh-subprocess"

var shippedDshProfiles = []string{"acp", "web", "headless", "sdk", "sdk-minimal"}

func dshHomeDir() string {
	if value := strings.TrimSpace(os.Getenv("DSH_HOME")); value != "" {
		if filepath.IsAbs(value) {
			return filepath.Clean(value)
		}
	}
	return filepath.Join(platform.GetRealHomeDir(), ".dsh")
}

func existingDshProfiles() []string {
	root := filepath.Join(dshHomeDir(), "profiles")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var profiles []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "desktop" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, entry.Name(), "package.json")); err == nil {
			profiles = append(profiles, entry.Name())
		}
	}
	sort.Strings(profiles)
	return profiles
}

func dshInstallProfiles() []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(shippedDshProfiles)+4)
	for _, profile := range append(append([]string(nil), shippedDshProfiles...), existingDshProfiles()...) {
		if _, ok := seen[profile]; ok {
			continue
		}
		seen[profile] = struct{}{}
		out = append(out, profile)
	}
	return out
}

func resolveDshSubprocessPluginPath() (string, error) {
	var candidates []string
	if configured := strings.TrimSpace(os.Getenv("AGENT_EBPF_DSH_PLUGIN_PATH")); configured != "" {
		candidates = append(candidates, configured)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "integrations", "dsh-subprocess"),
			filepath.Join(cwd, "..", "integrations", "dsh-subprocess"),
		)
	}
	if executable, err := os.Executable(); err == nil {
		dir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(dir, "integrations", "dsh-subprocess"),
			filepath.Join(dir, "..", "integrations", "dsh-subprocess"),
		)
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(absolute, "package.json")); err == nil {
			return absolute, nil
		}
	}
	return "", errors.New("DeepSeek Harness subprocess plugin package was not found; set AGENT_EBPF_DSH_PLUGIN_PATH or install from a repository build that includes integrations/dsh-subprocess")
}

func runDshPluginCommand(profile string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	commandArgs := []string{"plugin", "--profile", profile}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, "dsh", commandArgs...)
	configureCommandForRealUser(cmd)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 4096 {
			message = message[len(message)-4096:]
		}
		if ctx.Err() != nil {
			return fmt.Errorf("dsh plugin --profile %s timed out: %w", profile, ctx.Err())
		}
		return fmt.Errorf("dsh plugin --profile %s failed: %w: %s", profile, err, message)
	}
	return nil
}

func installDshSubprocessPlugin() error {
	packagePath, err := resolveDshSubprocessPluginPath()
	if err != nil {
		return err
	}
	var errs []error
	for _, profile := range dshInstallProfiles() {
		if err := runDshPluginCommand(profile, "add", packagePath); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func uninstallDshSubprocessPlugin() error {
	var errs []error
	for _, profile := range existingDshProfiles() {
		if !dshProfileHasPlugin(profile) {
			continue
		}
		if err := runDshPluginCommand(profile, "remove", dshSubprocessPluginPackage); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func dshProfileHasPlugin(profile string) bool {
	raw, err := os.ReadFile(filepath.Join(dshHomeDir(), "profiles", profile, "package.json"))
	if err != nil {
		return false
	}
	var manifest struct {
		Dependencies     map[string]string `json:"dependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return false
	}
	for _, dependencies := range []map[string]string{manifest.Dependencies, manifest.DevDependencies, manifest.PeerDependencies} {
		if _, ok := dependencies[dshSubprocessPluginPackage]; ok {
			return true
		}
	}
	return false
}

func isDshSubprocessPluginInstalled() bool {
	profiles := existingDshProfiles()
	if len(profiles) == 0 {
		return false
	}
	for _, profile := range profiles {
		if !dshProfileHasPlugin(profile) {
			return false
		}
	}
	return true
}
