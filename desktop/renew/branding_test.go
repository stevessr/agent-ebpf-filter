package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDesktopPublicBrand(t *testing.T) {
	if desktopBrandName != "明镜高悬" {
		t.Fatalf("unexpected desktop brand: %q", desktopBrandName)
	}
	if !strings.Contains(desktopWindowTitle, desktopBrandName) {
		t.Fatalf("window title does not carry the public brand: %q", desktopWindowTitle)
	}
	if desktopBrandSlogan == "" || desktopBrandDescription == "" {
		t.Fatal("public brand needs a meaningful slogan and functional description")
	}
}

func TestLegacyMyGoPackagingIdentityIsPreserved(t *testing.T) {
	data, err := os.ReadFile("mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name       string `json:"name"`
		Identifier string `json:"identifier"`
		Linux      struct {
			Command string `json:"command"`
			Comment string `json:"comment"`
		} `json:"linux"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "Renew" ||
		manifest.Identifier != "io.github.stevessr.agent-ebpf-filter.renew" ||
		manifest.Linux.Command != "agent-ebpf-renew" {
		t.Fatalf("legacy MyGo package identity changed: %+v", manifest)
	}
	if !strings.Contains(manifest.Linux.Comment, desktopBrandName) {
		t.Fatalf("desktop description omits public brand: %q", manifest.Linux.Comment)
	}
}
