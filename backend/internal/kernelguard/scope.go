package kernelguard

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// ScopeEvidence describes the coverage of one explicit deployment port.
// A static destination-port classifier has no Agent intent: a matched benign
// connection to an approved port would also be blocked in real operation.
type ScopeEvidence struct {
	Port             uint32
	MatchedAttacks   int
	MatchedAttackRun int
	MatchedBenign    int
}

// AssessScopeEvidence requires each approved port to have independent attack
// evidence and *no observed* benign collisions in the held-out split.
// It is a release prerequisite, not a statistical proof of future safety.
func AssessScopeEvidence(e ScopeEvidence) error {
	if e.Port == 0 || e.Port > 65535 || e.MatchedAttacks < 20 ||
		e.MatchedAttackRun < 10 || e.MatchedBenign > 0 {
		return fmt.Errorf("port %d lacks independent attack evidence or collides with benign flows: attacks=%d attack_runs=%d benign_collisions=%d",
			e.Port, e.MatchedAttacks, e.MatchedAttackRun, e.MatchedBenign)
	}
	return nil
}

// VerifyScopeCoverageCSV refuses to arm a model whose evaluation success
// came from a different port than the actual operator-approved scope.
// Input is independently parsed from the source CSV, not from the report.
func VerifyScopeCoverageCSV(path string, model Model, reviewedPorts []uint32) error {
	if err := model.Validate(); err != nil {
		return err
	}
	if len(reviewedPorts) == 0 || len(reviewedPorts) > 64 {
		return errors.New("at least one and at most 64 reviewed ports required")
	}
	ports := make(map[uint32]*ScopeEvidence, len(reviewedPorts))
	runs := make(map[uint32]map[string]bool, len(reviewedPorts))
	for _, port := range reviewedPorts {
		if port == 0 || port > 65535 || ports[port] != nil {
			return fmt.Errorf("invalid/duplicate port in scope: %d", port)
		}
		ports[port] = &ScopeEvidence{Port: port}
		runs[port] = make(map[string]bool)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) > 32<<20 {
		return errors.New("validation CSV exceeds 32MiB")
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = 4
	header, err := r.Read()
	if err != nil {
		return err
	}
	fields := make(map[string]int, 4)
	for i, name := range header {
		if _, exists := fields[name]; exists {
			return errors.New("duplicate header field")
		}
		fields[name] = i
	}
	for _, k := range []string{"agent_run_id", "destination_ip", "destination_port", "label"} {
		if _, ok := fields[k]; !ok {
			return fmt.Errorf("missing %s column", k)
		}
	}
	for line := 2; ; line++ {
		if line > 250002 {
			return errors.New("CSV row cap exceeded")
		}
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("CSV row %d: %w", line, err)
		}
		run := strings.TrimSpace(row[fields["agent_run_id"]])
		if run == "" || len(run) > 128 {
			return fmt.Errorf("invalid run id on row %d", line)
		}
		split := sha256.Sum256([]byte(run))
		if split[0] >= 51 {
			continue
		}
		portVal, err := strconv.ParseUint(strings.TrimSpace(row[fields["destination_port"]]), 10, 16)
		if err != nil || portVal == 0 {
			return fmt.Errorf("invalid destination port on row %d", line)
		}
		port := uint32(portVal)
		info := ports[port]
		if info == nil {
			continue
		}
		ip, err := netip.ParseAddr(strings.TrimSpace(row[fields["destination_ip"]]))
		if err != nil {
			return fmt.Errorf("invalid IP on row %d", line)
		}
		label := strings.ToLower(strings.TrimSpace(row[fields["label"]]))
		if label != "attack" && label != "benign" {
			return fmt.Errorf("invalid label on row %d", line)
		}
		if !Predict(model, ip, uint16(port)) {
			continue
		}
		if label == "benign" {
			info.MatchedBenign++
		} else {
			info.MatchedAttacks++
			runs[port][run] = true
		}
	}
	for _, port := range reviewedPorts {
		info := ports[port]
		info.MatchedAttackRun = len(runs[port])
		if err := AssessScopeEvidence(*info); err != nil {
			return err
		}
	}
	return nil
}
