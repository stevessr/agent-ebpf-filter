package dshcli

import (
	"path/filepath"
	"strings"
)

const (
	ModeLauncher          = "launcher"
	ModeProfile           = "profile"
	ModePlugin            = "plugin"
	ModeDumpConfig        = "dump-config"
	ModeDumpDefaultConfig = "dump-default-config"
	ModeDumpConfigSchema  = "dump-config-schema"
)

// Invocation is the privacy-safe launcher metadata Agent eBPF needs from a dsh command.
// It intentionally excludes app arguments, prompts, package names, and patch paths.
type Invocation struct {
	Mode      string
	Profile   string
	Operation string
}

// IsCommand reports whether comm names the DeepSeek Harness CLI.
func IsCommand(comm string) bool {
	return filepath.Base(strings.TrimSpace(comm)) == "dsh"
}

// Parse mirrors only dsh's public launcher boundary. Unknown tokens terminate
// launcher parsing because the real dsh CLI forwards that suffix verbatim to
// the selected profile.
func Parse(args []string) Invocation {
	if len(args) == 0 {
		return Invocation{Mode: ModeLauncher}
	}
	if args[0] == "plugin" {
		return parsePlugin(args[1:])
	}

	inv := Invocation{Mode: ModeLauncher}
	dumpMode := ""
	index := 0
	if !strings.HasPrefix(args[0], "-") {
		inv.Profile = strings.TrimSpace(args[0])
		index = 1
	}

	for index < len(args) {
		arg := args[index]
		switch {
		case arg == "--profile":
			if index+1 >= len(args) {
				return finalizeProfileInvocation(inv, dumpMode)
			}
			inv.Profile = strings.TrimSpace(args[index+1])
			index += 2
		case strings.HasPrefix(arg, "--profile="):
			inv.Profile = strings.TrimSpace(strings.TrimPrefix(arg, "--profile="))
			index++
		case arg == "--patch" || arg == "--from-default-profile":
			if index+1 >= len(args) {
				return finalizeProfileInvocation(inv, dumpMode)
			}
			index += 2
		case strings.HasPrefix(arg, "--patch=") || strings.HasPrefix(arg, "--from-default-profile="):
			index++
		case arg == "--dump-config":
			dumpMode = ModeDumpConfig
			index++
		case arg == "--dump-default-config":
			dumpMode = ModeDumpDefaultConfig
			index++
		case arg == "--dump-config-schema":
			dumpMode = ModeDumpConfigSchema
			index++
		case arg == "-V" || arg == "--version":
			return finalizeProfileInvocation(inv, dumpMode)
		default:
			// dsh gives the first unrecognized token and the complete suffix to
			// the profile app. Do not inspect it here.
			return finalizeProfileInvocation(inv, dumpMode)
		}
	}

	return finalizeProfileInvocation(inv, dumpMode)
}

func finalizeProfileInvocation(inv Invocation, dumpMode string) Invocation {
	if inv.Profile == "" {
		inv.Mode = ModeLauncher
		return inv
	}
	if dumpMode != "" {
		inv.Mode = dumpMode
		return inv
	}
	inv.Mode = ModeProfile
	return inv
}

func parsePlugin(args []string) Invocation {
	inv := Invocation{Mode: ModePlugin}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--profile":
			if index+1 < len(args) {
				inv.Profile = strings.TrimSpace(args[index+1])
				index++
			}
		case strings.HasPrefix(arg, "--profile="):
			inv.Profile = strings.TrimSpace(strings.TrimPrefix(arg, "--profile="))
		}
	}

	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--profile" {
			index++
			continue
		}
		if strings.HasPrefix(arg, "--profile=") || strings.HasPrefix(arg, "-") {
			continue
		}
		if operation := normalizePluginOperation(arg); operation != "" {
			inv.Operation = operation
			break
		}
		// The first forwarded positional is pnpm's command. Keep arbitrary
		// values out of telemetry rather than recording a package/script name.
		inv.Operation = "pnpm"
		break
	}
	return inv
}

func normalizePluginOperation(value string) string {
	switch strings.TrimSpace(value) {
	case "add", "remove", "rm", "uninstall", "update", "up", "install", "i", "why", "list", "ls",
		"allow-version", "revoke-version", "version-exemptions":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}
