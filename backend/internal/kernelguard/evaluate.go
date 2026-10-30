package kernelguard

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

type HoldoutCounts struct {
	Benign            int
	Attacks           int
	FalsePositives    int
	TruePositives     int
	IndependentGroups int
}

// EvaluateCSVHoldout recomputes the Python trainer's deterministic run split
// and decisions in Go. It cannot attest that labels are truthful, but prevents
// a report for another model or data file from silently arming enforcement.
func EvaluateCSVHoldout(path string, model Model) (HoldoutCounts, error) {
	var result HoldoutCounts
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if len(data) > 32<<20 {
		return result, errors.New("validation CSV exceeds 32MiB")
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = 4
	header, err := r.Read()
	if err != nil {
		return result, err
	}
	order := make(map[string]int, 4)
	for i, field := range header {
		if _, duplicate := order[field]; duplicate {
			return result, errors.New("duplicate validation CSV header")
		}
		order[field] = i
	}
	for _, name := range []string{"agent_run_id", "destination_ip", "destination_port", "label"} {
		if _, ok := order[name]; !ok {
			return result, fmt.Errorf("missing CSV column %s", name)
		}
	}
	groups := make(map[string]bool)
	for line := 2; ; line++ {
		if line > 250002 {
			return result, errors.New("validation CSV row cap exceeded")
		}
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return result, fmt.Errorf("CSV row %d: %w", line, err)
		}
		run := strings.TrimSpace(rec[order["agent_run_id"]])
		if run == "" || len(run) > 128 {
			return result, fmt.Errorf("CSV row %d invalid run id", line)
		}
		portStr := strings.TrimSpace(rec[order["destination_port"]])
		portInt, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil || portInt == 0 {
			return result, fmt.Errorf("CSV row %d invalid port", line)
		}
		ip, err := netip.ParseAddr(strings.TrimSpace(rec[order["destination_ip"]]))
		if err != nil {
			return result, fmt.Errorf("CSV row %d invalid destination", line)
		}
		label := strings.ToLower(strings.TrimSpace(rec[order["label"]]))
		if label != "attack" && label != "benign" {
			return result, fmt.Errorf("CSV row %d invalid label", line)
		}
		// Exactly the same group split as train-kernel-guard.py. Only the
		// holdout rows count towards release evidence.
		digest := sha256.Sum256([]byte(run))
		if digest[0] >= 51 {
			continue
		}
		groups[run] = true
		matched := Predict(model, ip, uint16(portInt))
		if label == "attack" {
			result.Attacks++
			if matched {
				result.TruePositives++
			}
		} else {
			result.Benign++
			if matched {
				result.FalsePositives++
			}
		}
	}
	result.IndependentGroups = len(groups)
	return result, nil
}

func VerifyHoldoutAgainstCSV(reportPath, csvPath string, model Model, digest string) error {
	// Reuse strict report schema/point gate, then independently rescore the
	// actual CSV, rather than trusting claimed confusion-matrix numbers.
	if err := ReadHoldout(reportPath, digest); err != nil {
		return err
	}
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		return err
	}
	var report Holdout
	if err := json.Unmarshal(reportData, &report); err != nil {
		return err
	}
	recomputed, err := EvaluateCSVHoldout(csvPath, model)
	if err != nil {
		return err
	}
	if report.Benign != recomputed.Benign ||
		report.Attacks != recomputed.Attacks ||
		report.FalsePositives != recomputed.FalsePositives ||
		report.TruePositives != recomputed.TruePositives ||
		report.IndependentGroups != recomputed.IndependentGroups {
		return fmt.Errorf("holdout counts differ from independently replayed CSV")
	}
	return nil
}
