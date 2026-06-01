package main

func seedDefaultTrackedCommands() {
	commonCLIs := map[string]string{
		"git": "Git",
		"npm": "Language Pkg", "bun": "Language Pkg", "pnpm": "Language Pkg",
		"yarn": "Language Pkg", "pip": "Language Pkg", "pip3": "Language Pkg",
		"gem": "Language Pkg", "uv": "Language Pkg", "zig": "Language Pkg",
		"dpkg": "System Pkg", "apt": "System Pkg", "apt-get": "System Pkg",
		"snap": "System Pkg", "flatpak": "System Pkg",
		"pacman": "System Pkg", "yay": "System Pkg", "paru": "System Pkg",
		"dnf": "System Pkg", "yum": "System Pkg", "zypper": "System Pkg",
		"rpm": "System Pkg", "nix": "System Pkg", "brew": "System Pkg",
		"docker": "Container CLI", "podman": "Container CLI", "kubectl": "Container CLI",
		"claude": "Agent CLI", "gemini": "Agent CLI", "codex": "Agent CLI",
		"kiro-cli": "Agent CLI", "gh": "Agent CLI", "cursor": "Agent CLI",
		"go": "Build Tool", "cargo": "Build Tool", "rustc": "Build Tool",
		"gcc": "Build Tool", "g++": "Build Tool", "clang": "Build Tool",
		"make": "Build Tool", "cmake": "Build Tool", "ninja": "Build Tool",
		"meson": "Build Tool", "gradle": "Build Tool", "mvn": "Build Tool",
		"lldb": "Build Tool", "gdb": "Build Tool",
		"node": "Runtime", "python": "Runtime", "python3": "Runtime",
		"java": "Runtime", "javac": "Runtime", "ruby": "Runtime",
		"perl": "Runtime", "lua": "Runtime", "deno": "Runtime", "pwsh": "Runtime",
		"php": "Runtime", "dotnet": "Runtime", "erl": "Runtime", "ghc": "Runtime",
		"systemctl": "System Tool", "journalctl": "System Tool",
		"ffmpeg": "System Tool", "tar": "System Tool", "gzip": "System Tool",
		"unzip": "System Tool",
		"ssh":   "Network Tool", "scp": "Network Tool", "rsync": "Network Tool",
		"curl": "Network Tool", "wget": "Network Tool",
		"bash": "Shell", "zsh": "Shell", "fish": "Shell",
		"sh": "Shell", "dash": "Shell", "ash": "Shell",
	}
	for cl, tag := range commonCLIs {
		var k [16]byte
		copy(k[:], cl)
		_ = trackerMaps.TrackedComms.Put(k, getTagID(tag))
	}
	for _, sh := range []string{"bash", "zsh", "fish", "sh", "dash", "ash"} {
		disabledComms[sh] = struct{}{}
	}
}
