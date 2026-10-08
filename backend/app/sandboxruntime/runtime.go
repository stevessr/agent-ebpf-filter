package sandboxruntime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

const defaultActiveLimit = 128

var (
	hexContainerID = regexp.MustCompile(`(?i)(?:^|/)(?:docker-|cri-containerd-)?([a-f0-9]{12,64})(?:\\.scope)?(?:$|/)`)
	plainHexID     = regexp.MustCompile(`(?i)^[a-f0-9]{12,64}$`)
)

type RuntimeDescriptor struct {
	Name                   string   `json:"name"`
	Kind                   string   `json:"kind"`
	Binaries               []string `json:"binaries"`
	DetectedBinaries       []string `json:"detectedBinaries,omitempty"`
	HostAttribution        bool     `json:"hostAttribution"`
	GuestSyscallsVisible   bool     `json:"guestSyscallsVisible"`
	GuestTelemetry         string   `json:"guestTelemetry,omitempty"`
	EnforcementIntegration string   `json:"enforcementIntegration,omitempty"`
}

type Status struct {
	Available         bool                `json:"available"`
	Platform          string              `json:"platform"`
	ProcRoot          string              `json:"procRoot"`
	SupportedRuntimes []RuntimeDescriptor `json:"supportedRuntimes"`
	Notes             []string            `json:"notes,omitempty"`
}

type Detection struct {
	PID                  int      `json:"pid"`
	Runtime              string   `json:"runtime,omitempty"`
	Kind                 string   `json:"kind,omitempty"`
	ContainerID          string   `json:"containerId,omitempty"`
	CgroupPath           string   `json:"cgroupPath,omitempty"`
	Comm                 string   `json:"comm,omitempty"`
	Command              string   `json:"command,omitempty"`
	Sources              []string `json:"sources,omitempty"`
	Confidence           float64  `json:"confidence,omitempty"`
	HostBoundary         bool     `json:"hostBoundary"`
	GuestSyscallsVisible bool     `json:"guestSyscallsVisible"`
	Notes                []string `json:"notes,omitempty"`
}

type runtimeRule struct {
	name            string
	kind            string
	binaries        []string
	processMarkers  []string
	cgroupMarkers   []string
	guestTelemetry  string
	enforcementNote string
	guestVisible    bool
}

var runtimeRules = []runtimeRule{
	{
		name:           "gVisor",
		kind:           "gvisor",
		binaries:       []string{"runsc"},
		processMarkers: []string{"runsc", "gvisor"},
		cgroupMarkers:  []string{"gvisor", "runsc"},
		guestTelemetry: "Use gVisor seccheck trace sessions with the remote Unix-socket sink for structured guest trace points; --strace/--strace-event are diagnostic alternatives.",
		enforcementNote: "Agent eBPF continues to enforce at the host cgroup/BPF-LSM boundary. Coverage depends on the gVisor platform/network mode; guest syscalls are implemented by Sentry and are not ordinary host syscalls.",
	},
	{
		name:           "Kata Containers",
		kind:           "kata",
		binaries:       []string{"containerd-shim-kata-v2", "kata-runtime", "kata-qemu", "kata-clh"},
		processMarkers: []string{"containerd-shim-kata-v2", "kata-runtime", "kata-qemu", "kata-clh"},
		cgroupMarkers:  []string{"kata"},
		guestTelemetry: "Host attribution covers shim/VMM processes. Guest-kernel telemetry requires a guest-side collector or runtime-specific channel.",
		enforcementNote: "Host cgroup/BPF-LSM policy applies to host-side runtime processes; VM guest policy requires a guest or VMM integration.",
	},
	{
		name:           "Firecracker",
		kind:           "firecracker",
		binaries:       []string{"firecracker", "jailer", "containerd-shim-aws-firecracker"},
		processMarkers: []string{"firecracker", "jailer", "containerd-shim-aws-firecracker"},
		cgroupMarkers:  []string{"firecracker"},
		guestTelemetry: "Host attribution covers Firecracker/jailer/shim processes. Guest syscalls require a guest-side collector.",
		enforcementNote: "Host cgroup/BPF-LSM policy applies outside the microVM; guest policy is a separate boundary.",
	},
	{
		name:           "Bubblewrap",
		kind:           "bubblewrap",
		binaries:       []string{"bwrap"},
		processMarkers: []string{"bwrap", "bubblewrap"},
		cgroupMarkers:  []string{"bwrap", "bubblewrap"},
		guestVisible:   true,
		enforcementNote: "The sandbox still executes Linux processes on the host kernel, so normal eBPF syscall tracing remains applicable subject to namespaces/cgroup attribution.",
	},
	{
		name:           "nsjail",
		kind:           "nsjail",
		binaries:       []string{"nsjail"},
		processMarkers: []string{"nsjail"},
		cgroupMarkers:  []string{"nsjail"},
		guestVisible:   true,
		enforcementNote: "The sandbox uses the host kernel; eBPF tracing and cgroup/BPF-LSM controls remain applicable to the host-visible processes.",
	},
	{
		name:           "runc / crun / youki",
		kind:           "oci",
		binaries:       []string{"runc", "crun", "youki", "containerd-shim-runc-v2"},
		processMarkers: []string{"containerd-shim-runc-v2", "runc", "crun", "youki"},
		cgroupMarkers:  []string{"docker", "containerd", "libpod", "kubepods"},
		guestVisible:   true,
		enforcementNote: "OCI namespace containers share the host kernel, so eBPF tracing and cgroup/BPF-LSM controls remain the primary integration.",
	},
}

type Manager struct {
	procRoot string
	lookPath func(string) (string, error)
}

func NewManager() *Manager {
	return &Manager{procRoot: "/proc", lookPath: exec.LookPath}
}

func newManagerForTest(procRoot string, lookPath func(string) (string, error)) *Manager {
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &Manager{procRoot: procRoot, lookPath: lookPath}
}

func (m *Manager) Status() Status {
	status := Status{
		Available: runtime.GOOS == "linux",
		Platform: runtime.GOOS,
		ProcRoot: m.procRoot,
		Notes: []string{
			"Runtime detection is best-effort host attribution based on /proc command lines and cgroups.",
			"gVisor Sentry implements guest syscalls in user space, so host syscall eBPF does not equal guest syscall telemetry.",
			"Container IDs discovered here reuse the existing Agent eBPF container_id attribution field.",
		},
	}
	for _, rule := range runtimeRules {
		desc := RuntimeDescriptor{
			Name:                   rule.name,
			Kind:                   rule.kind,
			Binaries:               append([]string(nil), rule.binaries...),
			HostAttribution:        true,
			GuestSyscallsVisible:   rule.guestVisible,
			GuestTelemetry:         rule.guestTelemetry,
			EnforcementIntegration: rule.enforcementNote,
		}
		for _, binary := range rule.binaries {
			if path, err := m.lookPath(binary); err == nil && path != "" {
				desc.DetectedBinaries = append(desc.DetectedBinaries, path)
			}
		}
		status.SupportedRuntimes = append(status.SupportedRuntimes, desc)
	}
	return status
}

func (m *Manager) DetectPID(pid int) (Detection, error) {
	if pid <= 0 {
		return Detection{}, fmt.Errorf("invalid pid %d", pid)
	}
	base := filepath.Join(m.procRoot, strconv.Itoa(pid))
	comm, commErr := os.ReadFile(filepath.Join(base, "comm"))
	cmdline, cmdErr := os.ReadFile(filepath.Join(base, "cmdline"))
	cgroup, cgErr := os.ReadFile(filepath.Join(base, "cgroup"))
	if commErr != nil && cmdErr != nil && cgErr != nil {
		return Detection{}, fmt.Errorf("read pid %d from %s: %w", pid, m.procRoot, errors.Join(commErr, cmdErr, cgErr))
	}
	return detectSnapshot(pid, string(comm), cmdline, string(cgroup)), nil
}

func (m *Manager) ListActive(limit int) ([]Detection, error) {
	if limit <= 0 {
		limit = defaultActiveLimit
	}
	if limit > 1024 {
		limit = 1024
	}
	entries, err := os.ReadDir(m.procRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", m.procRoot, err)
	}
	out := make([]Detection, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		d, err := m.DetectPID(pid)
		if err != nil || d.Runtime == "" {
			continue
		}
		out = append(out, d)
		if len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out, nil
}

func detectSnapshot(pid int, commRaw string, cmdlineRaw []byte, cgroupRaw string) Detection {
	comm := strings.TrimSpace(commRaw)
	args := splitCmdline(cmdlineRaw)
	command := strings.Join(args, " ")
	cgroupPath := selectCgroupPath(cgroupRaw)

	d := Detection{
		PID:          pid,
		Comm:         comm,
		Command:      command,
		CgroupPath:   cgroupPath,
		HostBoundary: true,
	}

	processText := strings.ToLower(strings.TrimSpace(comm + " " + command))
	cgroupText := strings.ToLower(cgroupPath)
	bestScore := 0.0
	var best runtimeRule
	for _, rule := range runtimeRules {
		score := 0.0
		for _, marker := range rule.processMarkers {
			if markerMatch(processText, strings.ToLower(marker)) {
				score = max(score, 0.92)
			}
		}
		for _, marker := range rule.cgroupMarkers {
			if strings.Contains(cgroupText, strings.ToLower(marker)) {
				score = max(score, 0.72)
			}
		}
		if score > bestScore {
			bestScore = score
			best = rule
		}
	}

	if bestScore > 0 {
		d.Runtime = best.name
		d.Kind = best.kind
		d.Confidence = bestScore
		d.GuestSyscallsVisible = best.guestVisible
		if bestScore >= 0.9 {
			d.Sources = append(d.Sources, "process")
		} else {
			d.Sources = append(d.Sources, "cgroup")
		}
		if best.kind == "gvisor" {
			d.Notes = append(d.Notes,
				"Host eBPF observes runsc/Sentry/Gofer host activity, not a one-to-one stream of guest syscalls.",
				"For guest syscall/trace-point telemetry use gVisor seccheck remote sinks (or diagnostic strace/event output).",
			)
		}
	}

	d.ContainerID = extractContainerID(args, cgroupPath)
	if d.ContainerID != "" {
		d.Sources = appendUnique(d.Sources, "container-id")
		if d.Confidence == 0 {
			d.Confidence = 0.55
		}
	}
	return d
}

func markerMatch(text, marker string) bool {
	if marker == "" {
		return false
	}
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '/' || r == '\\' || r == ':' || r == ','
	}) {
		if token == marker || filepath.Base(token) == marker {
			return true
		}
	}
	return strings.Contains(text, marker)
}

func splitCmdline(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	parts := strings.Split(string(raw), "\x00")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func selectCgroupPath(raw string) string {
	best := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		path := strings.TrimSpace(parts[2])
		if len(path) > len(best) {
			best = path
		}
	}
	return best
}

func extractContainerID(args []string, cgroupPath string) string {
	if id := containerIDFromArgs(args); id != "" {
		return id
	}
	if match := hexContainerID.FindStringSubmatch(cgroupPath); len(match) == 2 {
		return strings.ToLower(match[1])
	}
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(cgroupPath)), ".scope")
	base = strings.TrimPrefix(base, "docker-")
	base = strings.TrimPrefix(base, "cri-containerd-")
	if plainHexID.MatchString(base) {
		return strings.ToLower(base)
	}
	return ""
}

func containerIDFromArgs(args []string) string {
	for i, arg := range args {
		switch arg {
		case "-id", "--id", "--container-id", "--container_id":
			if i+1 < len(args) {
				return normalizeContainerID(args[i+1])
			}
		default:
			for _, prefix := range []string{"--id=", "-id=", "--container-id=", "--container_id="} {
				if strings.HasPrefix(arg, prefix) {
					return normalizeContainerID(strings.TrimPrefix(arg, prefix))
				}
			}
		}
	}
	for i := len(args) - 1; i >= 0; i-- {
		token := strings.TrimSpace(args[i])
		if token == "" || strings.HasPrefix(token, "-") {
			continue
		}
		if plainHexID.MatchString(token) {
			return strings.ToLower(token)
		}
	}
	return ""
}

func normalizeContainerID(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(value, ".scope"))
	value = strings.TrimPrefix(value, "docker-")
	value = strings.TrimPrefix(value, "cri-containerd-")
	if plainHexID.MatchString(value) {
		return strings.ToLower(value)
	}
	if value != "" && !strings.ContainsAny(value, "/\\ \t\r\n") && len(value) <= 128 {
		return value
	}
	return ""
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}
