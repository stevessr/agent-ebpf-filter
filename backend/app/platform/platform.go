package platform

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

func OriginalInvokerIDs() (uid, gid uint32, ok bool) {
	if uidStr := os.Getenv("SUDO_UID"); uidStr != "" {
		gidStr := os.Getenv("SUDO_GID")
		if gidStr == "" { return 0, 0, false }
		pUid, e1 := strconv.ParseUint(uidStr, 10, 32)
		pGid, e2 := strconv.ParseUint(gidStr, 10, 32)
		if e1 != nil || e2 != nil { return 0, 0, false }
		return uint32(pUid), uint32(pGid), true
	}
	if uidStr := os.Getenv("PKEXEC_UID"); uidStr != "" {
		u, err := user.LookupId(uidStr)
		if err != nil { return 0, 0, false }
		pUid, e1 := strconv.ParseUint(uidStr, 10, 32)
		pGid, e2 := strconv.ParseUint(u.Gid, 10, 32)
		if e1 != nil || e2 != nil { return 0, 0, false }
		return uint32(pUid), uint32(pGid), true
	}
	return 0, 0, false
}

func WriteFileAsRealUser(path string, data []byte, perm os.FileMode) error {
	if err := os.WriteFile(path, data, perm); err != nil { return err }
	if os.Getuid() == 0 {
		if uid, gid, ok := OriginalInvokerIDs(); ok { _ = os.Chown(path, int(uid), int(gid)) }
	}
	return nil
}

func MkdirAllAsRealUser(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil { return err }
	if os.Getuid() == 0 {
		if uid, gid, ok := OriginalInvokerIDs(); ok { _ = os.Chown(path, int(uid), int(gid)) }
	}
	return nil
}

var (
	getRealHomeOnce sync.Once
	getRealHomeVal  string
)

func GetRealHomeDir() string {
	getRealHomeOnce.Do(func() {
		if h := os.Getenv("AGENT_REAL_HOME"); h != "" { getRealHomeVal = h; return }
		if os.Getuid() == 0 {
			for _, env := range []string{"SUDO_USER", "PKEXEC_UID"} {
				if v := os.Getenv(env); v != "" {
					if u, err := user.Lookup(v); err == nil { getRealHomeVal = u.HomeDir; return }
				}
			}
			if home := os.Getenv("HOME"); home != "" && home != "/root" { getRealHomeVal = home; return }
		}
		h, _ := os.UserHomeDir()
		if h == "" || h == "/root" {
			if entries, err := os.ReadDir("/home"); err == nil {
				for _, e := range entries {
					if e.IsDir() && e.Name() != "lost+found" { getRealHomeVal = filepath.Join("/home", e.Name()); return }
				}
			}
		}
		getRealHomeVal = h
	})
	return getRealHomeVal
}

func RuntimeSettingsDir() string { return filepath.Join(GetRealHomeDir(), ".config", "agent-ebpf-filter") }
func RuntimeSettingsPath() string { return filepath.Join(RuntimeSettingsDir(), "runtime.json") }
func DefaultEventLogPath() string { return filepath.Join(RuntimeSettingsDir(), "events.jsonl") }

func FirstNonEmpty(values ...string) string {
	for _, v := range values { if strings.TrimSpace(v) != "" { return strings.TrimSpace(v) } }
	return ""
}

func ParseStringField(extraInfo, key string) string {
	needle := key + "="
	for _, part := range strings.Fields(strings.ReplaceAll(extraInfo, ",", " ")) {
		if strings.HasPrefix(part, needle) { return strings.TrimSpace(strings.TrimPrefix(part, needle)) }
	}
	return ""
}

func ParseUintField(extraInfo, key string) uint32 {
	val := ParseStringField(extraInfo, key)
	if val == "" { return 0 }
	var r uint64
	fmt.Sscanf(val, "%d", &r)
	return uint32(r)
}

func ParseFloatField(extraInfo, key string) float64 {
	val := ParseStringField(extraInfo, key)
	if val == "" { return 0 }
	var r float64
	fmt.Sscanf(val, "%f", &r)
	return r
}

func PluginsRootDir() string { return filepath.Join(RuntimeSettingsDir(), "plugins") }
func PluginDir(id string) string { return filepath.Join(PluginsRootDir(), id) }
func PluginManifestPath(id string) string { return filepath.Join(PluginDir(id), "manifest.json") }
func PluginSourcePath(id string) string { return filepath.Join(PluginDir(id), "source.c") }
func PluginObjectPath(id string) string { return filepath.Join(PluginDir(id), "program.o") }
func WritePluginSource(id, source string) error {
	if err := MkdirAllAsRealUser(PluginDir(id), 0755); err != nil { return err }
	return WriteFileAsRealUser(PluginSourcePath(id), []byte(source), 0644)
}
