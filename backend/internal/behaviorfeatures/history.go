package behaviorfeatures

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
)

type Report struct {
	Outputs     []Output `json:"outputs"`
	Duplicates  int      `json:"duplicates"`
	UnknownJoin int      `json:"unknown_join"`
}
type scope struct {
	Host, Boot, Run, PIDNS, Clock string
	TGID, Start                   uint64
}

func scopeOf(c Context) scope {
	return scope{c.HostID, c.BootID, c.RunID, c.PIDNS, c.Clock, c.TGID, c.StartTimeNS}
}

type dedupeKey struct {
	Scope      scope
	Source, ID string
}

// ExtractBatch adds prior-only counts over a bounded batch. A zero count means
// no preceding matching event IN THIS BATCH, not complete capture of the host.
// Source-local IDs deduplicate retransmits; different sources remain separate.
// No path equality, syscall-to-request join, or ordering proves data exfiltration.
func ExtractBatch(inputs []Input) (Report, error) {
	r := Report{Outputs: []Output{}}
	if len(inputs) > MaxEvents {
		return r, errors.New("batch exceeds event limit")
	}
	seen := map[dedupeKey][32]byte{}
	kept := []Input{}
	groups := map[scope][]int{}
	totalBytes := 0
	for inputIndex, i := range inputs {
		if !validInput(i) {
			return Report{}, errors.New("invalid or oversized behavior record")
		}
		b, e := json.Marshal(i)
		if e != nil {
			return Report{}, errors.New("invalid behavior record")
		}
		totalBytes += len(b)
		if totalBytes > MaxBatchBytes {
			return Report{}, errors.New("batch exceeds byte limit")
		}
		o, e := Extract(i)
		if e != nil {
			return Report{}, e
		}
		o.InputIndex = &inputIndex
		if i.ID != "" && i.Source != "" && complete(i.Context) {
			key := dedupeKey{scopeOf(i.Context), i.Source, i.ID}
			hash := sha256.Sum256(b)
			if old, ok := seen[key]; ok {
				if hash != old {
					return Report{}, errors.New("conflicting per-source event ID")
				}
				r.Duplicates++
				continue
			}
			seen[key] = hash
		}
		idx := len(kept)
		kept = append(kept, i)
		r.Outputs = append(r.Outputs, o)
		if complete(i.Context) {
			s := scopeOf(i.Context)
			groups[s] = append(groups[s], idx)
		} else {
			r.UnknownJoin++
		}
	}
	for _, ids := range groups {
		sort.SliceStable(ids, func(a, b int) bool { return kept[ids[a]].Context.EventTimeNS < kept[ids[b]].Context.EventTimeNS })
		sums := make([][4]int, len(ids)+1)
		for j, idx := range ids {
			sums[j+1] = sums[j]
			sums[j+1][0]++
			f := r.Outputs[idx].Features
			yes := func(k string) bool { return f[k].Present && f[k].Value == 1 }
			if yes("sys.op_read") && yes("sys.success") && f["sys.actual_bytes"].Value > 0 && yes("path.sensitive_hint") {
				sums[j+1][1]++
			}
			if yes("sys.op_write") && yes("sys.success") && f["sys.actual_bytes"].Value > 0 {
				sums[j+1][2]++
			}
			if yes("http.outgoing") {
				sums[j+1][3]++
			}
		}
		for _, idx := range ids {
			t := kept[idx].Context.EventTimeNS
			end := sort.Search(len(ids), func(j int) bool { return kept[ids[j]].Context.EventTimeNS >= t })
			for w, seconds := range map[string]int64{"1s": 1, "30s": 30, "300s": 300} {
				begin := sort.Search(end, func(j int) bool { return kept[ids[j]].Context.EventTimeNS >= t-seconds*1e9 })
				for n, k := range []string{"prior_events", "prior_sensitive_reads", "prior_writes", "prior_http_outgoing"} {
					r.Outputs[idx].put("history."+w+"."+k, float64(sums[end][n]-sums[begin][n]))
				}
			}
		}
	}
	return r, nil
}
