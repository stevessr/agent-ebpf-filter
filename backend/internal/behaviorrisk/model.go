package behaviorrisk

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"sync/atomic"

	"agent-ebpf-filter/internal/behaviorfeatures"
	"agent-ebpf-filter/internal/strictjson"
)

const MaxModelBytes = 2 * 1024 * 1024

type Spec struct {
	Encoding   string    `json:"encoding"`
	Kind       string    `json:"kind"`
	Weights    []float64 `json:"weights"`
	Intercept  float64   `json:"intercept"`
	Threshold  float64   `json:"threshold"`
	ShadowOnly bool      `json:"shadow_only"`
}
type Model struct {
	spec   Spec
	digest string
}
type Result struct {
	Kind        string   `json:"kind"`
	Mode        string   `json:"mode"`
	Status      string   `json:"status"`
	Score       *float64 `json:"score,omitempty"`
	ModelDigest string   `json:"model_digest,omitempty"`
}

func Load(r io.Reader, expectedSHA string) (*Model, error) {
	fail := errors.New("invalid model or checksum")
	expected, e := hex.DecodeString(expectedSHA)
	if e != nil || len(expected) != 32 {
		return nil, fail
	}
	b, e := io.ReadAll(io.LimitReader(r, MaxModelBytes+1))
	if e != nil || len(b) > MaxModelBytes {
		return nil, fail
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != expectedSHA {
		return nil, fail
	}
	var s Spec
	if strictjson.Decode(b, &s) != nil {
		return nil, fail
	}
	if s.Encoding != Encoding || (s.Kind != "http_parameter" && s.Kind != "http_request") || len(s.Weights) != Dimensions || !s.ShadowOnly || !finite(s.Intercept) || math.Abs(s.Intercept) > 1e6 || !finite(s.Threshold) || s.Threshold <= 0 || s.Threshold >= 1 {
		return nil, fail
	}
	for _, w := range s.Weights {
		if !finite(w) || math.Abs(w) > 1e6 {
			return nil, fail
		}
	}
	return &Model{spec: s, digest: expectedSHA}, nil
}
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func (m *Model) Score(i behaviorfeatures.Input) Result {
	r := Result{Mode: "shadow_only", Status: "abstain"}
	if m == nil {
		return r
	}
	r.ModelDigest = m.digest
	r.Kind = m.spec.Kind
	text, kind, e := Text(i)
	if e != nil || kind != m.spec.Kind {
		return r
	}
	v, e := Encode(text, kind)
	if e != nil {
		return r
	}
	z := m.spec.Intercept
	for _, entry := range v.Entries {
		z += m.spec.Weights[entry.Index] * entry.Value
	}
	score := 0.
	if z >= 0 {
		score = 1 / (1 + math.Exp(-z))
	} else {
		ez := math.Exp(z)
		score = ez / (1 + ez)
	}
	if !finite(score) {
		return r
	}
	r.Score = &score
	r.Status = "below_threshold"
	if score > m.spec.Threshold {
		r.Status = "above_threshold"
	}
	return r
}

// Manager atomically swaps only verified models. A failed reload retains the
// previous snapshot. Disable is an immediate rollback to abstention.
// There is deliberately no ALLOW/BLOCK result or enforcement callback.
type Manager struct{ current atomic.Pointer[Model] }

func (m *Manager) Reload(r io.Reader, sha string) error {
	model, e := Load(r, sha)
	if e != nil {
		return e
	}
	m.current.Store(model)
	return nil
}
func (m *Manager) Disable()                              { m.current.Store(nil) }
func (m *Manager) Score(i behaviorfeatures.Input) Result { return m.current.Load().Score(i) }
