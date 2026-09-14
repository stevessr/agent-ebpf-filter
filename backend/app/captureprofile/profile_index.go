package captureprofile

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

const maxProfileDispatchFanout = 4096

type dispatchKey struct {
	source    string
	protocol  string
	direction string
	method    string
	transport string
}

type compiledProfile struct {
	profile     Profile
	remoteCIDRs []netip.Prefix
}

type RegistryStats struct {
	Generation      uint64 `json:"generation"`
	Profiles        int    `json:"profiles"`
	DispatchBuckets int    `json:"dispatchBuckets"`
	DispatchEntries int    `json:"dispatchEntries"`
	MaxBucket       int    `json:"maxBucket"`
}

type profileDispatchIndex struct {
	compiled []compiledProfile
	buckets  map[dispatchKey][]int
	stats    RegistryStats
}

type preparedObservation struct {
	Observation
	remoteAddr netip.Addr
}

func prepareObservation(observation Observation) preparedObservation {
	observation = normalizeObservation(observation)
	prepared := preparedObservation{Observation: observation}
	if observation.RemoteIP != "" {
		if addr, err := netip.ParseAddr(observation.RemoteIP); err == nil {
			prepared.remoteAddr = addr.Unmap()
			if observation.Family == "" {
				if prepared.remoteAddr.Is4() {
					prepared.Family = "ipv4"
				} else if prepared.remoteAddr.Is6() {
					prepared.Family = "ipv6"
				}
			}
		}
	}
	return prepared
}

func compileProfile(profile Profile) (compiledProfile, error) {
	compiled := compiledProfile{profile: profile}
	for _, port := range profile.RemotePorts {
		if port == 0 || port > 65535 {
			return compiledProfile{}, fmt.Errorf("API profile %q remote port %d is outside 1..65535", profile.ID, port)
		}
	}
	if len(profile.RemoteCIDRs) != 0 {
		compiled.remoteCIDRs = make([]netip.Prefix, 0, len(profile.RemoteCIDRs))
		for _, raw := range profile.RemoteCIDRs {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
			if err != nil {
				return compiledProfile{}, fmt.Errorf("API profile %q invalid remote CIDR %q: %w", profile.ID, raw, err)
			}
			compiled.remoteCIDRs = append(compiled.remoteCIDRs, prefix.Masked())
		}
		sort.Slice(compiled.remoteCIDRs, func(i, j int) bool {
			if compiled.remoteCIDRs[i].Addr().Compare(compiled.remoteCIDRs[j].Addr()) != 0 {
				return compiled.remoteCIDRs[i].Addr().Compare(compiled.remoteCIDRs[j].Addr()) < 0
			}
			return compiled.remoteCIDRs[i].Bits() < compiled.remoteCIDRs[j].Bits()
		})
	}
	return compiled, nil
}

func dispatchValues(values []string) []string {
	if len(values) == 0 {
		return []string{""}
	}
	return values
}

func compileProfileDispatch(profiles []Profile) (*profileDispatchIndex, error) {
	index := &profileDispatchIndex{
		compiled: make([]compiledProfile, len(profiles)),
		buckets:  make(map[dispatchKey][]int),
	}
	for profileIndex, profile := range profiles {
		compiled, err := compileProfile(profile)
		if err != nil {
			return nil, err
		}
		index.compiled[profileIndex] = compiled
		dimensions := [][]string{
			dispatchValues(profile.Sources),
			dispatchValues(profile.Protocols),
			dispatchValues(profile.Directions),
			dispatchValues(profile.Methods),
			dispatchValues(profile.Transports),
		}
		fanout := 1
		for _, values := range dimensions {
			fanout *= len(values)
			if fanout > maxProfileDispatchFanout {
				return nil, fmt.Errorf("API profile %q expands to %d dispatch buckets (max %d)", profile.ID, fanout, maxProfileDispatchFanout)
			}
		}
		for _, source := range dimensions[0] {
			for _, protocol := range dimensions[1] {
				for _, direction := range dimensions[2] {
					for _, method := range dimensions[3] {
						for _, transport := range dimensions[4] {
							key := dispatchKey{source: source, protocol: protocol, direction: direction, method: method, transport: transport}
							index.buckets[key] = append(index.buckets[key], profileIndex)
						}
					}
				}
			}
		}
	}
	index.stats.Profiles = len(profiles)
	index.stats.DispatchBuckets = len(index.buckets)
	for _, bucket := range index.buckets {
		index.stats.DispatchEntries += len(bucket)
		if len(bucket) > index.stats.MaxBucket {
			index.stats.MaxBucket = len(bucket)
		}
	}
	return index, nil
}

func candidateValues(value string) ([2]string, int) {
	if value == "" {
		return [2]string{"", ""}, 1
	}
	return [2]string{value, ""}, 2
}

func dispatchCandidateKeys(observation preparedObservation) ([32]dispatchKey, int) {
	var keys [32]dispatchKey
	sources, sourceN := candidateValues(observation.Source)
	protocols, protocolN := candidateValues(observation.Protocol)
	directions, directionN := candidateValues(observation.Direction)
	methods, methodN := candidateValues(observation.Method)
	transports, transportN := candidateValues(observation.Transport)
	n := 0
	for si := 0; si < sourceN; si++ {
		for pi := 0; pi < protocolN; pi++ {
			for di := 0; di < directionN; di++ {
				for mi := 0; mi < methodN; mi++ {
					for ti := 0; ti < transportN; ti++ {
						keys[n] = dispatchKey{
							source: sources[si], protocol: protocols[pi], direction: directions[di],
							method: methods[mi], transport: transports[ti],
						}
						n++
					}
				}
			}
		}
	}
	return keys, n
}

func matchIndexedSnapshot(snapshot *profileSnapshot, observation Observation, detailed bool) (Match, bool) {
	prepared := prepareObservation(observation)
	keys, count := dispatchCandidateKeys(prepared)
	best := Match{}
	matched := false
	for i := 0; i < count; i++ {
		for _, profileIndex := range snapshot.index.buckets[keys[i]] {
			candidate, ok := matchCompiledProfile(&snapshot.index.compiled[profileIndex], &prepared, detailed)
			if !ok {
				continue
			}
			if !matched || candidate.Score > best.Score || (candidate.Score == best.Score && candidate.ProfileID < best.ProfileID) {
				best = candidate
				matched = true
			}
		}
	}
	return best, matched
}

func (r *Registry) MatchAll(observation Observation) []Match {
	if r == nil {
		return nil
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil || snapshot.index == nil {
		return nil
	}
	prepared := prepareObservation(observation)
	keys, count := dispatchCandidateKeys(prepared)
	matches := make([]Match, 0, 4)
	for i := 0; i < count; i++ {
		for _, profileIndex := range snapshot.index.buckets[keys[i]] {
			if candidate, ok := matchCompiledProfile(&snapshot.index.compiled[profileIndex], &prepared, true); ok {
				matches = append(matches, candidate)
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].ProfileID < matches[j].ProfileID
	})
	return matches
}

func (r *Registry) Stats() RegistryStats {
	if r == nil {
		return RegistryStats{}
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil || snapshot.index == nil {
		return RegistryStats{}
	}
	return snapshot.index.stats
}

func containsExact(items []string, value string) bool {
	if len(items) == 0 {
		return true
	}
	if value == "" {
		return false
	}
	index := sort.SearchStrings(items, value)
	return index < len(items) && items[index] == value
}

func containsPort(items []uint32, value uint32) bool {
	if len(items) == 0 {
		return true
	}
	if value == 0 {
		return false
	}
	index := sort.Search(len(items), func(i int) bool { return items[i] >= value })
	return index < len(items) && items[index] == value
}

func matchCompiledProfile(compiled *compiledProfile, observation *preparedObservation, detailed bool) (Match, bool) {
	profile := &compiled.profile
	score := 0
	var matched uint32
	const (
		matchedSource uint32 = 1 << iota
		matchedProtocol
		matchedDirection
		matchedMethod
		matchedHost
		matchedPath
		matchedContentType
		matchedTransport
		matchedFamily
		matchedRemotePort
		matchedRemoteCIDR
		matchedProcess
	)

	if len(profile.Sources) != 0 {
		if !containsExact(profile.Sources, observation.Source) {
			return Match{}, false
		}
		score += 3
		matched |= matchedSource
	}
	if len(profile.Protocols) != 0 {
		if !containsExact(profile.Protocols, observation.Protocol) {
			return Match{}, false
		}
		score += 5
		matched |= matchedProtocol
	}
	if len(profile.Directions) != 0 {
		if !containsExact(profile.Directions, observation.Direction) {
			return Match{}, false
		}
		score += 3
		matched |= matchedDirection
	}
	if len(profile.Methods) != 0 {
		if !containsExact(profile.Methods, observation.Method) {
			return Match{}, false
		}
		score += 5
		matched |= matchedMethod
	}
	if len(profile.Transports) != 0 {
		if !containsExact(profile.Transports, observation.Transport) {
			return Match{}, false
		}
		score += 3
		matched |= matchedTransport
	}
	if len(profile.Families) != 0 {
		if !containsExact(profile.Families, observation.Family) {
			return Match{}, false
		}
		score += 3
		matched |= matchedFamily
	}
	if len(profile.RemotePorts) != 0 {
		if !containsPort(profile.RemotePorts, observation.RemotePort) {
			return Match{}, false
		}
		score += 8
		matched |= matchedRemotePort
	}
	if len(compiled.remoteCIDRs) != 0 {
		if !observation.remoteAddr.IsValid() {
			return Match{}, false
		}
		cidrMatch := false
		for _, prefix := range compiled.remoteCIDRs {
			if prefix.Contains(observation.remoteAddr) {
				cidrMatch = true
				break
			}
		}
		if !cidrMatch {
			return Match{}, false
		}
		score += 12
		matched |= matchedRemoteCIDR
	}
	if len(profile.Processes) != 0 {
		if !containsExact(profile.Processes, observation.Process) {
			return Match{}, false
		}
		score += 5
		matched |= matchedProcess
	}

	hostMatched := false
	if observation.Host != "" {
		for _, suffix := range profile.HostSuffixes {
			if hostSuffixMatchNormalized(observation.Host, suffix) {
				hostMatched = true
				break
			}
		}
		if !hostMatched {
			for _, fragment := range profile.HostContains {
				if strings.Contains(observation.Host, fragment) {
					hostMatched = true
					break
				}
			}
		}
	}
	if len(profile.HostSuffixes)+len(profile.HostContains) != 0 && !hostMatched {
		return Match{}, false
	}
	if hostMatched {
		score += 60
		matched |= matchedHost
	}

	pathMatched := false
	if observation.Path != "" {
		for _, prefix := range profile.PathPrefixes {
			if strings.HasPrefix(observation.Path, prefix) {
				pathMatched = true
				break
			}
		}
		if !pathMatched {
			for _, fragment := range profile.PathContains {
				if strings.Contains(observation.Path, fragment) {
					pathMatched = true
					break
				}
			}
		}
	}
	if len(profile.PathPrefixes)+len(profile.PathContains) != 0 && !pathMatched {
		return Match{}, false
	}
	if pathMatched {
		score += 30
		matched |= matchedPath
	}

	for _, header := range profile.RequiredHeaders {
		if !hasHeader(observation.Headers, header) {
			return Match{}, false
		}
		score += 5
	}
	if len(profile.ContentTypes) != 0 {
		contentTypeMatch := false
		for _, contentType := range profile.ContentTypes {
			if strings.Contains(observation.ContentType, contentType) {
				contentTypeMatch = true
				break
			}
		}
		if !contentTypeMatch {
			return Match{}, false
		}
		score += 5
		matched |= matchedContentType
	}
	if score < profile.MinScore {
		return Match{}, false
	}

	confidence := uint32(score)
	if confidence > 100 {
		confidence = 100
	}
	if !detailed {
		return Match{
			ProfileID: profile.ID, Vendor: profile.Vendor, Product: profile.Product, Operation: profile.Operation,
			Confidence: confidence, Score: score,
		}, true
	}

	matchedBy := make([]string, 0, 12+len(profile.RequiredHeaders))
	fields := []struct {
		bit   uint32
		label string
	}{
		{matchedSource, "source"}, {matchedProtocol, "protocol"}, {matchedDirection, "direction"},
		{matchedMethod, "method"}, {matchedTransport, "transport"}, {matchedFamily, "family"},
		{matchedRemotePort, "remote-port"}, {matchedRemoteCIDR, "remote-cidr"}, {matchedProcess, "process"},
		{matchedHost, "host"}, {matchedPath, "path"}, {matchedContentType, "content-type"},
	}
	for _, field := range fields {
		if matched&field.bit != 0 {
			matchedBy = append(matchedBy, field.label)
		}
	}
	for _, header := range profile.RequiredHeaders {
		matchedBy = append(matchedBy, "header:"+header)
	}
	return Match{
		ProfileID: profile.ID, Vendor: profile.Vendor, Product: profile.Product, Operation: profile.Operation,
		Confidence: confidence, Score: score, MatchedBy: matchedBy,
	}, true
}
