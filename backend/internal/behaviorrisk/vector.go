// Package behaviorrisk provides bounded, task-specific shadow scoring. It has
// no authorization or enforcement API. Raw text is consumed, never returned.
package behaviorrisk

import (
	"errors"
	"math"
	"net/url"
	"sort"
	"strings"

	"agent-ebpf-filter/internal/behaviorfeatures"
)

const Encoding = "byte-fnv1a-3-5-logtf-l2.v1"
const Dimensions = 32768
const MaxBytes = 16384

type Entry struct {
	Index int     `json:"index"`
	Value float64 `json:"value"`
}
type Vector struct {
	Kind    string  `json:"kind"`
	Entries []Entry `json:"entries"`
}

// Text supplies a deterministic bounded representation. HTTP capture must
// explicitly be complete; syscall events have no trained head and are refused.
func Text(i behaviorfeatures.Input) (string, string, error) {
	if err := behaviorfeatures.Validate(i); err != nil {
		return "", "", err
	}
	if i.CaptureLoss != nil && *i.CaptureLoss {
		return "", "", errors.New("capture_loss")
	}
	if i.Parameter != nil {
		if len(*i.Parameter) > MaxBytes {
			return "", "", errors.New("text_limit")
		}
		return *i.Parameter, "http_parameter", nil
	}
	if i.HTTP == nil {
		return "", "", errors.New("unsupported_task")
	}
	h := i.HTTP
	if h.Truncated == nil || *h.Truncated || h.Target == nil || h.Method == "" || !h.HeadersPresent || h.Body == nil {
		return "", "", errors.New("incomplete_http")
	}
	// Bound the concatenation before allocating it. No field is silently truncated.
	n := len(h.Method) + len(*h.Target) + len(*h.Body) + 3
	for _, v := range h.Headers {
		n += len(v.Name) + len(v.Value) + 2
	}
	if n > MaxBytes {
		return "", "", errors.New("text_limit")
	}
	var b strings.Builder
	b.Grow(n)
	b.WriteString(h.Method)
	b.WriteByte('\n')
	b.WriteString(*h.Target)
	b.WriteByte('\n')
	for _, v := range h.Headers {
		b.WriteString(strings.ToLower(v.Name))
		b.WriteByte(':')
		b.WriteString(v.Value)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(*h.Body)
	return b.String(), "http_request", nil
}

func Encode(text, kind string) (Vector, error) {
	if len(text) > MaxBytes {
		return Vector{}, errors.New("text_limit")
	}
	if kind != "http_parameter" && kind != "http_request" {
		return Vector{}, errors.New("unsupported_task")
	}
	counts := make(map[int]int)
	decoded := text
	for n := 0; n < 2; n++ {
		v, e := url.QueryUnescape(decoded)
		if e != nil || v == decoded {
			break
		}
		decoded = v
	}
	// Raw and decoded views have separate hash namespaces. This is lexical
	// modeling only: decoding never changes parsed HTTP/path boundaries.
	for view, s := range []string{text, decoded} {
		data := []byte(s)
		for j, c := range data {
			if c >= 'A' && c <= 'Z' {
				data[j] = c + 32
			}
		}
		for start := range data {
			h := uint32(2166136261) ^ uint32(view+1)
			for width := 1; width <= 5 && start+width <= len(data); width++ {
				h ^= uint32(data[start+width-1])
				h *= 16777619
				if width >= 3 {
					counts[int(h%Dimensions)]++
				}
			}
		}
	}
	out := Vector{Kind: kind, Entries: make([]Entry, 0, len(counts))}
	for index, count := range counts {
		value := math.Log1p(float64(count))
		out.Entries = append(out.Entries, Entry{index, value})
	}
	// Sum in index order: Go map iteration must not affect floating-point output.
	sort.Slice(out.Entries, func(a, b int) bool { return out.Entries[a].Index < out.Entries[b].Index })
	norm := 0.
	for _, entry := range out.Entries {
		norm += entry.Value * entry.Value
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for j := range out.Entries {
			out.Entries[j].Value /= norm
		}
	}
	return out, nil
}
