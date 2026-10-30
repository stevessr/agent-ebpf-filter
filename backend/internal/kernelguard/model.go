// Package kernelguard defines a fixed-point model contract shared with the
// opt-in eBPF cgroup program. It is independent of the general 128-dim ML
// feature extractor: OS socket hooks cannot access private prompts or history.
package kernelguard

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"
)

const SchemaVersion = "kernel-cgroup-linear-v1"
const FeatureCount = 6
const ModeShadow uint32 = 1
const ModeEnforce uint32 = 2

var FeatureNames = [FeatureCount]string{
	"external_ipv4_candidate", "port_22", "port_4444",
	"port_ge_49152", "port_443", "port_53",
}

// Model is a bounded signed-integer linear score, not a calibrated
// probability. The input classifier is deterministic and self-contained.
type Model struct {
	SchemaVersion string              `json:"schemaVersion"`
	Generation    uint32              `json:"generation"`
	Bias          int32               `json:"bias"`
	Threshold     int32               `json:"threshold"`
	Weights       [FeatureCount]int32 `json:"weights"`
}

// MapConfig must match kernel_ml_guard.c:struct guard_config exactly.
type MapConfig struct {
	Version    uint32
	Mode       uint32
	Generation uint32
	Bias       int32
	Threshold  int32
	Weights    [FeatureCount]int32
}

func (m Model) Config(mode uint32) MapConfig {
	return MapConfig{Version: 1, Mode: mode, Generation: m.Generation,
		Bias: m.Bias, Threshold: m.Threshold, Weights: m.Weights}
}

func (m Model) Validate() error {
	if m.SchemaVersion != SchemaVersion || m.Generation == 0 ||
		m.Bias < -4096 || m.Bias > 4096 ||
		m.Threshold < 1 || m.Threshold > 8192 {
		return errors.New("invalid schema, generation, bias or threshold")
	}
	portSpecific := false
	baseline := int64(m.Bias) + int64(m.Weights[0])
	for i, w := range m.Weights {
		if w < -2048 || w > 2048 {
			return fmt.Errorf("weight %d outside [-2048,2048]", i)
		}
		if i > 0 && w > 0 {
			portSpecific = true
		}
	}
	if !portSpecific || baseline >= int64(m.Threshold) {
		return errors.New("model must require a positive port feature; generic external traffic must not match")
	}
	return nil
}

func ReadModel(path string) (Model, string, error) {
	var m Model
	data, err := os.ReadFile(path)
	if err != nil {
		return m, "", err
	}
	if len(data) > 16*1024 {
		return m, "", errors.New("model exceeds 16KiB")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return m, "", err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return m, "", errors.New("trailing JSON or multiple documents")
	}
	if err := m.Validate(); err != nil {
		return m, "", err
	}
	sum := sha256.Sum256(data)
	return m, hex.EncodeToString(sum[:]), nil
}

// ExternalIPv4Candidate mirrors the bounded kernel predicate. It is a safety
// gate, not a substitute for routing, NAT, destination reputation or policy.
func ExternalIPv4Candidate(ip netip.Addr) bool {
	if !ip.Is4() {
		return false
	}
	b := ip.As4()
	a, second, third := b[0], b[1], b[2]
	if a == 0 || a == 10 || a == 127 || a >= 224 {
		return false
	}
	if a == 100 && second >= 64 && second <= 127 {
		return false
	}
	if a == 169 && second == 254 {
		return false
	}
	if a == 172 && second >= 16 && second <= 31 {
		return false
	}
	if a == 192 && (second == 168 || (second == 0 && third == 2)) {
		return false
	}
	if a == 198 && ((second == 18 || second == 19) || (second == 51 && third == 100)) {
		return false
	}
	if a == 203 && second == 0 && third == 113 {
		return false
	}
	return true
}

func Features(ip netip.Addr, port uint16) [FeatureCount]int32 {
	var f [FeatureCount]int32
	if !ExternalIPv4Candidate(ip) {
		return f
	}
	f[0] = 1
	if port == 22 {
		f[1] = 1
	}
	if port == 4444 {
		f[2] = 1
	}
	if port >= 49152 {
		f[3] = 1
	}
	if port == 443 {
		f[4] = 1
	}
	if port == 53 {
		f[5] = 1
	}
	return f
}

func Score(m Model, f [FeatureCount]int32) int32 {
	score := m.Bias
	for i := 0; i < FeatureCount; i++ {
		score += m.Weights[i] * f[i]
	}
	return score
}

func Predict(m Model, ip netip.Addr, port uint16) bool {
	if !ExternalIPv4Candidate(ip) || port == 0 {
		return false
	}
	return Score(m, Features(ip, port)) >= m.Threshold
}

type Holdout struct {
	SchemaVersion     string `json:"schemaVersion"`
	ModelSHA256       string `json:"modelSHA256"`
	Benign            int    `json:"benign"`
	Attacks           int    `json:"attacks"`
	FalsePositives    int    `json:"falsePositives"`
	TruePositives     int    `json:"truePositives"`
	IndependentGroups int    `json:"independentGroups"`
}

// AssessHoldout checks numerical release gates, not authenticity of the input
// dataset or evaluator. The operator must still independently review provenance.
func AssessHoldout(h Holdout, hash string) error {
	if h.SchemaVersion != "kernel-guard-holdout-v1" ||
		!strings.EqualFold(h.ModelSHA256, hash) || len(hash) != 64 {
		return errors.New("holdout schema/digest mismatch")
	}
	if h.Benign < 1000 || h.Attacks < 100 || h.IndependentGroups < 10 ||
		h.FalsePositives < 0 || h.TruePositives < 0 ||
		h.FalsePositives > h.Benign || h.TruePositives > h.Attacks {
		return errors.New("holdout evidence insufficient or invalid")
	}
	// <= 0.1% point FPR and >= 95% recall; not a statistical confidence bound.
	if int64(h.FalsePositives)*1000 > int64(h.Benign) ||
		int64(h.TruePositives)*100 < 95*int64(h.Attacks) {
		return errors.New("holdout does not meet point FPR / recall gates")
	}
	return nil
}

func ReadHoldout(path string, modelHash string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) > 16*1024 {
		return errors.New("holdout exceeds 16KiB")
	}
	var h Holdout
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&h); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing holdout JSON")
	}
	return AssessHoldout(h, modelHash)
}
