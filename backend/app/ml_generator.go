package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/platform"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// healthGeneratorPIDs tracks PIDs designated for health dataset generation.
// Key: uint32 (PID), Value: time.Time (Registration Timestamp)
var healthGeneratorPIDs sync.Map

type processInfo struct {
	PID     uint32 `json:"pid"`
	Name    string `json:"name"`
	Cmdline string `json:"cmdline"`
	User    string `json:"user"`
}

// getRunningProcesses lists running processes in the system by scanning /proc
func getRunningProcesses() ([]processInfo, error) {
	files, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var list []processInfo
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		pid, err := strconv.ParseUint(f.Name(), 10, 32)
		if err != nil {
			continue
		}

		// Read process name from /proc/<pid>/comm
		nameBytes, err := os.ReadFile(filepath.Join("/proc", f.Name(), "comm"))
		name := ""
		if err == nil {
			name = strings.TrimSpace(string(nameBytes))
		}

		// Read cmdline
		cmdBytes, err := os.ReadFile(filepath.Join("/proc", f.Name(), "cmdline"))
		cmdline := ""
		if err == nil {
			// /proc/<pid>/cmdline is null-separated
			cmdline = strings.ReplaceAll(string(cmdBytes), "\x00", " ")
			cmdline = strings.TrimSpace(cmdline)
		}
		if cmdline == "" && name != "" {
			cmdline = "[" + name + "]"
		}

		// Read user info from owner uid of stat
		var username string
		if statInfo, err := os.Stat(filepath.Join("/proc", f.Name())); err == nil {
			if stat, ok := statInfo.Sys().(*syscall.Stat_t); ok {
				uidStr := strconv.FormatUint(uint64(stat.Uid), 10)
				if u, err := user.LookupId(uidStr); err == nil {
					username = u.Username
				} else {
					username = uidStr
				}
			}
		}

		list = append(list, processInfo{
			PID:     uint32(pid),
			Name:    name,
			Cmdline: cmdline,
			User:    username,
		})
	}
	return list, nil
}

// handleMLHealthProcessesGet returns a list of system processes for attachment
func handleMLHealthProcessesGet(c *gin.Context) {
	list, err := getRunningProcesses()
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to read processes: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"processes": list})
}

// handleMLHealthGeneratorsGet lists all registered health generators
func handleMLHealthGeneratorsGet(c *gin.Context) {
	var list []gin.H
	healthGeneratorPIDs.Range(func(key, val interface{}) bool {
		pid := key.(uint32)
		regTime := val.(time.Time)

		// Verify process details
		name := "Dead Process"
		cmdline := ""
		username := ""
		
		if statInfo, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
			if nameBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
				name = strings.TrimSpace(string(nameBytes))
			}
			if cmdBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
				cmdline = strings.ReplaceAll(string(cmdBytes), "\x00", " ")
				cmdline = strings.TrimSpace(cmdline)
			}
			if stat, ok := statInfo.Sys().(*syscall.Stat_t); ok {
				uidStr := strconv.FormatUint(uint64(stat.Uid), 10)
				if u, err := user.LookupId(uidStr); err == nil {
					username = u.Username
				} else {
					username = uidStr
				}
			}
		}

		list = append(list, gin.H{
			"pid":          pid,
			"name":         name,
			"cmdline":      cmdline,
			"user":         username,
			"registeredAt": regTime.Format(time.RFC3339),
		})
		return true
	})

	c.JSON(200, gin.H{"generators": list})
}

// handleMLHealthRegisterPost registers a PID for health data collection
func handleMLHealthRegisterPost(c *gin.Context) {
	var req struct {
		PID uint32 `json:"pid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}

	// Double check if process folder exists in proc
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", req.PID)); err != nil {
		c.JSON(404, gin.H{"error": "process not found"})
		return
	}

	// Register in AgentPids so eBPF tracks it
	if trackerMaps.AgentPids != nil {
		if err := trackerMaps.AgentPids.Put(req.PID, getTagID("Health Generator")); err != nil {
			c.JSON(500, gin.H{"error": "failed to register in eBPF map: " + err.Error()})
			return
		}
	}

	// Set process context in tracker
	name := "HealthProcess"
	if nameBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", req.PID)); err == nil {
		name = strings.TrimSpace(string(nameBytes))
	}
	trackedProcessContexts.Set(req.PID, events.ProcessContext{
		RootAgentPid: req.PID,
		ToolName:     name,
		AgentRunID:   "health-generator-" + strconv.FormatUint(uint64(req.PID), 10),
	})

	healthGeneratorPIDs.Store(req.PID, time.Now())
	c.JSON(200, gin.H{"status": "ok"})
}

// handleMLHealthUnregisterPost removes a PID from health data collection
func handleMLHealthUnregisterPost(c *gin.Context) {
	var req struct {
		PID uint32 `json:"pid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}

	if trackerMaps.AgentPids != nil {
		_ = trackerMaps.AgentPids.Delete(req.PID)
	}
	trackedProcessContexts.Delete(req.PID)
	healthGeneratorPIDs.Delete(req.PID)

	c.JSON(200, gin.H{"status": "ok"})
}

// handleMLHealthRunPost starts a command manually wrapped in agent-wrapper
// and registers the new child process as a health generator.
func handleMLHealthRunPost(c *gin.Context) {
	var req struct {
		Comm string   `json:"comm"`
		Args []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Comm == "" {
		c.JSON(400, gin.H{"error": "invalid run request (comm is required)"})
		return
	}

	wb := platform.ResolveWrapperPath()
	if wb == "" {
		c.JSON(500, gin.H{"error": "agent-wrapper executable not found"})
		return
	}

	// Construct wrapped command
	cmd := exec.Command(wb, append([]string{req.Comm}, req.Args...)...)
	cmd.Env = os.Environ()
	dropPrivileges(cmd)

	if err := cmd.Start(); err != nil {
		c.JSON(500, gin.H{"error": "failed to start process: " + err.Error()})
		return
	}

	pid := uint32(cmd.Process.Pid)

	// Register PID in eBPF map
	if trackerMaps.AgentPids != nil {
		_ = trackerMaps.AgentPids.Put(pid, getTagID("Health Generator"))
	}

	// Register in process context
	trackedProcessContexts.Set(pid, events.ProcessContext{
		RootAgentPid: pid,
		ToolName:     req.Comm,
		AgentRunID:   "health-generator-" + strconv.FormatUint(uint64(pid), 10),
	})

	// Add to health generator sync map
	healthGeneratorPIDs.Store(pid, time.Now())

	c.JSON(200, gin.H{
		"status": "started",
		"pid":    pid,
		"comm":   req.Comm,
	})
}
