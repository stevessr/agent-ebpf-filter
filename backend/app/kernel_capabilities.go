package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf/btf"
	"github.com/gin-gonic/gin"
)

type kernelCapabilities struct {
	Release          string   `json:"release"`
	ReleaseError     string   `json:"releaseError,omitempty"`
	BTFReadable      bool     `json:"btfReadable"`
	BTFError         string   `json:"btfError,omitempty"`
	LSMs             []string `json:"lsms"`
	LSMError         string   `json:"lsmError,omitempty"`
	BPFLSMEnabled    bool     `json:"bpfLsmEnabled"`
	InstalledKernels []string `json:"installedKernels"`
	KernelsError     string   `json:"kernelsError,omitempty"`
}

// Reads only fixed kernel paths. Missing securityfs or permissions are unknown,
// not proof that the kernel lacks LSM support. Module directories are inventory,
// not proof that a bootable kernel image or its BTF is installed.
func inspectKernelCapabilities(root string, loadBTF func() error) kernelCapabilities {
	result := kernelCapabilities{LSMs: []string{}, InstalledKernels: []string{}}
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(root, name)) }
	if data, err := read("proc/sys/kernel/osrelease"); err != nil {
		result.ReleaseError = err.Error()
	} else {
		result.Release = strings.TrimSpace(string(data))
	}
	if err := loadBTF(); err != nil {
		result.BTFError = err.Error()
	} else {
		result.BTFReadable = true
	}
	if data, err := read("sys/kernel/security/lsm"); err != nil {
		result.LSMError = err.Error()
	} else {
		for _, name := range strings.Split(strings.TrimSpace(string(data)), ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			result.LSMs = append(result.LSMs, name)
			if name == "bpf" {
				result.BPFLSMEnabled = true
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "lib/modules"))
	if err != nil {
		result.KernelsError = err.Error()
	} else {
		for _, entry := range entries {
			if entry.IsDir() {
				result.InstalledKernels = append(result.InstalledKernels, entry.Name())
			}
		}
	}
	return result
}

func handleKernelCapabilities(c *gin.Context) {
	c.JSON(200, inspectKernelCapabilities("/", func() error {
		// Parse the live file, rather than relying on the process-wide kernel BTF cache.
		_, err := btf.LoadSpec("/sys/kernel/btf/vmlinux")
		return err
	}))
}
