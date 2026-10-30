// kernel-ml-guard loads a separate, transient IPv4 and IPv4-mapped IPv6 cgroup model. It never
// attaches to the host root cgroup or changes existing sandbox policy maps.
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"agent-ebpf-filter/internal/kernelguard"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type counters struct {
	Examined uint64
	Reviewed uint64
	Matched  uint64
	Blocked  uint64
}

func parsePorts(s string) ([]uint32, error) {
	if strings.TrimSpace(s) == "" {
		return nil, errors.New("specify 1-64 operator-reviewed ports")
	}
	parts := strings.Split(s, ",")
	if len(parts) > 64 {
		return nil, errors.New("too many reviewed ports")
	}
	seen := make(map[uint32]bool, len(parts))
	out := make([]uint32, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		n, err := strconv.ParseUint(part, 10, 16)
		if err != nil || n == 0 {
			return nil, fmt.Errorf("invalid port: %q", part)
		}
		p := uint32(n)
		if seen[p] {
			return nil, fmt.Errorf("duplicate port %d", p)
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

func scopeCgroup(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolved = filepath.Clean(resolved)
	if resolved == "/sys/fs/cgroup" || resolved == "/" ||
		!strings.HasPrefix(resolved, "/sys/fs/cgroup/") {
		return "", errors.New("refusing root or out-of-tree cgroup")
	}
	st, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", errors.New("cgroup path is not a directory")
	}
	return resolved, nil
}

// validateCollectionSpec verifies the compiled ELF contract *before*
// allocating kernel resources or attaching to a cgroup. BPF ELF compilation
// alone cannot guarantee correct map layout or intended hook attachment.
func validateCollectionSpec(spec *ebpf.CollectionSpec) error {
	if spec == nil {
		return errors.New("nil BPF collection spec")
	}
	mapChecks := []struct {
		name                string
		kind                ebpf.MapType
		key, value, entries uint32
	}{
		{"kernel_guard_config", ebpf.Array, 4, uint32(binary.Size(kernelguard.MapConfig{})), 1},
		{"kernel_guard_ports", ebpf.Hash, 4, 4, 64},
		{"kernel_guard_stats", ebpf.PerCPUArray, 4, uint32(binary.Size(counters{})), 1},
	}
	for _, expected := range mapChecks {
		m := spec.Maps[expected.name]
		if m == nil || m.Type != expected.kind || m.KeySize != expected.key ||
			m.ValueSize != expected.value || m.MaxEntries != expected.entries {
			return fmt.Errorf("unexpected BPF map ABI for %s", expected.name)
		}
	}
	programChecks := []struct {
		name   string
		attach ebpf.AttachType
	}{
		{"kernel_guard_connect4", ebpf.AttachCGroupInet4Connect},
		{"kernel_guard_sendmsg4", ebpf.AttachCGroupUDP4Sendmsg},
		{"kernel_guard_connect6", ebpf.AttachCGroupInet6Connect},
		{"kernel_guard_sendmsg6", ebpf.AttachCGroupUDP6Sendmsg},
	}
	for _, expected := range programChecks {
		prog := spec.Programs[expected.name]
		if prog == nil || prog.Type != ebpf.CGroupSockAddr ||
			prog.AttachType != expected.attach {
			return fmt.Errorf("unexpected BPF program/attach ABI for %s", expected.name)
		}
	}
	return nil
}

func run(ctx context.Context, object, modelPath, evidence, validationCSV, cgroup, portsCSV, mode, approvedDigest string) error {
	if mode != "shadow" && mode != "block" {
		return errors.New("mode must be shadow or block")
	}
	scoped, err := scopeCgroup(cgroup)
	if err != nil {
		return err
	}
	ports, err := parsePorts(portsCSV)
	if err != nil {
		return err
	}
	m, digest, err := kernelguard.ReadModel(modelPath)
	if err != nil {
		return err
	}

	// Approval is independent of classifier predictions. The report is a
	// numerical gate, not cryptographic proof of dataset provenance.
	block := mode == "block"
	if block {
		if len(approvedDigest) != 64 || !strings.EqualFold(approvedDigest, digest) {
			return fmt.Errorf("operator must pass --approve-sha256 %s", digest)
		}
		if evidence == "" || validationCSV == "" {
			return errors.New("block requires --holdout and --validation-csv")
		}
		if err := kernelguard.VerifyHoldoutAgainstCSV(evidence, validationCSV, m, digest); err != nil {
			return fmt.Errorf("holdout gate: %w", err)
		}
		if err := kernelguard.VerifyScopeCoverageCSV(validationCSV, m, ports); err != nil {
			return fmt.Errorf("reviewed port scope gate: %w", err)
		}
	}

	spec, err := ebpf.LoadCollectionSpec(object)
	if err != nil {
		return fmt.Errorf("load BPF ELF: %w", err)
	}
	if err := validateCollectionSpec(spec); err != nil {
		return fmt.Errorf("BPF ELF ABI preflight: %w", err)
	}
	col, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("load BPF programs: %w", err)
	}
	defer col.Close()
	config := col.Maps["kernel_guard_config"]
	reviewed := col.Maps["kernel_guard_ports"]
	stats := col.Maps["kernel_guard_stats"]
	c4 := col.Programs["kernel_guard_connect4"]
	s4 := col.Programs["kernel_guard_sendmsg4"]
	c6 := col.Programs["kernel_guard_connect6"]
	s6 := col.Programs["kernel_guard_sendmsg6"]
	if config == nil || reviewed == nil || stats == nil || c4 == nil || s4 == nil || c6 == nil || s6 == nil {
		return errors.New("BPF object missing required maps/programs")
	}
	key := uint32(0)
	cfg := m.Config(kernelguard.ModeShadow) // attach and validate *before* arming
	if err := config.Put(&key, &cfg); err != nil {
		return err
	}
	for _, port := range ports {
		enabled := uint32(1)
		if err := reviewed.Put(&port, &enabled); err != nil {
			return err
		}
	}
	connLink, err := link.AttachCgroup(link.CgroupOptions{
		Path: scoped, Attach: ebpf.AttachCGroupInet4Connect, Program: c4,
	})
	if err != nil {
		return fmt.Errorf("attach IPv4 connect: %w", err)
	}
	defer connLink.Close()
	sendLink, err := link.AttachCgroup(link.CgroupOptions{
		Path: scoped, Attach: ebpf.AttachCGroupUDP4Sendmsg, Program: s4,
	})
	if err != nil {
		return fmt.Errorf("attach IPv4 UDP sendmsg: %w", err)
	}
	defer sendLink.Close()
	conn6, err := link.AttachCgroup(link.CgroupOptions{
		Path: scoped, Attach: ebpf.AttachCGroupInet6Connect, Program: c6,
	})
	if err != nil {
		return fmt.Errorf("attach IPv6 connect (IPv4-mapped only): %w", err)
	}
	defer conn6.Close()
	send6, err := link.AttachCgroup(link.CgroupOptions{
		Path: scoped, Attach: ebpf.AttachCGroupUDP6Sendmsg, Program: s6,
	})
	if err != nil {
		return fmt.Errorf("attach IPv6 UDP sendmsg (IPv4-mapped only): %w", err)
	}
	defer send6.Close()

	if block {
		cfg = m.Config(kernelguard.ModeEnforce)
		if err := config.Put(&key, &cfg); err != nil {
			return err
		}
	}
	log.Printf("guard active scope=%s mode=%s generation=%d model_sha256=%s approved_ports=%v",
		scoped, mode, m.Generation, digest, ports)
	log.Print("Native IPv6 (non-IPv4-mapped), established TCP, other cgroups and unlisted ports are NOT covered")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Flip back to shadow before detaching.
			shadow := m.Config(kernelguard.ModeShadow)
			if err := config.Put(&key, &shadow); err != nil {
				log.Printf("disarm: %v", err)
			}
			return nil
		case <-ticker.C:
			cpus, err := ebpf.PossibleCPU()
			if err != nil {
				log.Printf("stats cpu count: %v", err)
				continue
			}
			values := make([]counters, cpus)
			if err := stats.Lookup(&key, &values); err != nil {
				log.Printf("stats lookup: %v", err)
				continue
			}
			var sum counters
			for _, x := range values {
				sum.Examined += x.Examined
				sum.Reviewed += x.Reviewed
				sum.Matched += x.Matched
				sum.Blocked += x.Blocked
			}
			log.Printf("guard stats examined=%d in_scope=%d matches=%d blocked=%d",
				sum.Examined, sum.Reviewed, sum.Matched, sum.Blocked)
		}
	}
}

func main() {
	var obj, model, holdout, validationCSV, cgroup, ports, mode, approved string
	var validateOnly bool
	flag.StringVar(&obj, "object", "", "compiled backend/ebpf/kernel_ml_guard.o")
	flag.StringVar(&model, "model", "", "validated linear model JSON")
	flag.StringVar(&holdout, "holdout", "", "group-held-out evaluation JSON, required for block")
	flag.StringVar(&validationCSV, "validation-csv", "", "source labeled CSV for independent Go holdout replay (block)")
	flag.StringVar(&cgroup, "cgroup", "", "non-root cgroup v2 directory")
	flag.StringVar(&ports, "ports", "", "comma-separated explicitly reviewed destination ports")
	flag.StringVar(&mode, "mode", "shadow", "shadow (default) or block")
	flag.StringVar(&approved, "approve-sha256", "", "operator-pinned exact model digest (block mode)")
	flag.BoolVar(&validateOnly, "validate-only", false, "offline model + group holdout verification; no BPF objects or root required")
	flag.Parse()
	if validateOnly {
		if model == "" || holdout == "" || validationCSV == "" {
			log.Fatal("--validate-only requires --model, --holdout and --validation-csv")
		}
		m, digest, err := kernelguard.ReadModel(model)
		if err != nil {
			log.Fatal(err)
		}
		if err := kernelguard.VerifyHoldoutAgainstCSV(holdout, validationCSV, m, digest); err != nil {
			log.Fatal(err)
		}
		if strings.TrimSpace(ports) != "" {
			reviewed, err := parsePorts(ports)
			if err != nil {
				log.Fatal(err)
			}
			if err := kernelguard.VerifyScopeCoverageCSV(validationCSV, m, reviewed); err != nil {
				log.Fatal(err)
			}
			log.Printf("offline holdout and reviewed port scopes validated for model_sha256=%s; no BPF program loaded", digest)
			return
		}
		log.Printf("offline holdout validated; port coverage NOT checked (pass --ports); model_sha256=%s", digest)
		return
	}
	if obj == "" || model == "" || cgroup == "" {
		log.Fatal("--object, --model and --cgroup are required")
	}
	// Reject a world-readable model approval file via an insecure implicit
	// default. Digest is confirmed on the CLI for each enforced invocation.
	// Do not expose these flags through untrusted Agent tool interfaces.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, obj, model, holdout, validationCSV, cgroup, ports, mode, approved); err != nil {
		log.Fatal(err)
	}
}
