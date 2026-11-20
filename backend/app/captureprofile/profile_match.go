package captureprofile

import (
	"sort"
	"strings"
)

func normalizeProfile(profile Profile) Profile {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Vendor = strings.TrimSpace(profile.Vendor)
	profile.Product = strings.TrimSpace(profile.Product)
	profile.Operation = strings.TrimSpace(profile.Operation)
	profile.Sources = normalizeList(profile.Sources)
	profile.Protocols = normalizeList(profile.Protocols)
	profile.Directions = normalizeList(profile.Directions)
	profile.Methods = normalizeUpperList(profile.Methods)
	profile.HostSuffixes = normalizeList(profile.HostSuffixes)
	profile.HostContains = normalizeList(profile.HostContains)
	profile.PathPrefixes = normalizePathList(profile.PathPrefixes)
	profile.PathContains = normalizePathList(profile.PathContains)
	profile.RequiredHeaders = normalizeList(profile.RequiredHeaders)
	profile.ContentTypes = normalizeList(profile.ContentTypes)
	profile.Transports = normalizeList(profile.Transports)
	profile.Families = normalizeList(profile.Families)
	profile.RemotePorts = normalizeUint32List(profile.RemotePorts)
	profile.RemoteCIDRs = normalizeList(profile.RemoteCIDRs)
	profile.Processes = normalizeList(profile.Processes)
	return profile
}

func normalizeObservation(observation Observation) Observation {
	observation.Source = strings.ToLower(strings.TrimSpace(observation.Source))
	observation.Protocol = strings.ToLower(strings.TrimSpace(observation.Protocol))
	observation.Direction = strings.ToLower(strings.TrimSpace(observation.Direction))
	observation.Method = strings.ToUpper(strings.TrimSpace(observation.Method))
	observation.Host = NormalizeHost(observation.Host)
	observation.Path = RequestPath(observation.Path)
	observation.ContentType = strings.ToLower(strings.TrimSpace(observation.ContentType))
	observation.Transport = strings.ToLower(strings.TrimSpace(observation.Transport))
	observation.Family = strings.ToLower(strings.TrimSpace(observation.Family))
	observation.RemoteIP = strings.TrimSpace(observation.RemoteIP)
	observation.Process = strings.ToLower(strings.TrimSpace(observation.Process))
	return observation
}

func matchProfile(profile Profile, observation Observation) (Match, bool) {
	score := 0
	matchedBy := make([]string, 0, 8)

	if len(profile.Sources) != 0 {
		if !containsFold(profile.Sources, observation.Source) {
			return Match{}, false
		}
		score += 3
		matchedBy = append(matchedBy, "source")
	}
	if len(profile.Protocols) != 0 {
		if !containsFold(profile.Protocols, observation.Protocol) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "protocol")
	}
	if len(profile.Directions) != 0 {
		if !containsFold(profile.Directions, observation.Direction) {
			return Match{}, false
		}
		score += 3
		matchedBy = append(matchedBy, "direction")
	}
	if len(profile.Methods) != 0 {
		if observation.Method == "" || !containsFold(profile.Methods, observation.Method) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "method")
	}

	hostMatched := false
	if observation.Host != "" {
		for _, suffix := range profile.HostSuffixes {
			if hostSuffixMatch(observation.Host, suffix) {
				hostMatched = true
				break
			}
		}
		if !hostMatched {
			for _, fragment := range profile.HostContains {
				if fragment != "" && strings.Contains(observation.Host, fragment) {
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
		matchedBy = append(matchedBy, "host")
	}

	pathMatched := false
	if observation.Path != "" {
		for _, prefix := range profile.PathPrefixes {
			if prefix != "" && strings.HasPrefix(observation.Path, prefix) {
				pathMatched = true
				break
			}
		}
		if !pathMatched {
			for _, fragment := range profile.PathContains {
				if fragment != "" && strings.Contains(observation.Path, fragment) {
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
		matchedBy = append(matchedBy, "path")
	}

	for _, header := range profile.RequiredHeaders {
		if !hasHeader(observation.Headers, header) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "header:"+header)
	}
	if len(profile.ContentTypes) != 0 {
		matchedContentType := false
		for _, contentType := range profile.ContentTypes {
			if contentType != "" && strings.Contains(observation.ContentType, contentType) {
				matchedContentType = true
				break
			}
		}
		if !matchedContentType {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "content-type")
	}
	if score < profile.MinScore {
		return Match{}, false
	}
	return Match{
		ProfileID:  profile.ID,
		Vendor:     profile.Vendor,
		Product:    profile.Product,
		Operation:  profile.Operation,
		Confidence: min(uint32(score), 100),
		Score:      score,
		MatchedBy:  matchedBy,
	}, true
}

func hostSuffixMatch(host, suffix string) bool {
	return hostSuffixMatchNormalized(NormalizeHost(host), NormalizeHost(suffix))
}

// hostSuffixMatchNormalized is the capture hot-path variant. Registry profiles
// and prepared observations are normalized before matching, so re-running URL,
// case and whitespace normalization for every candidate only burns CPU.
func hostSuffixMatchNormalized(host, suffix string) bool {
	if host == "" || suffix == "" {
		return false
	}
	if strings.HasPrefix(suffix, ".") {
		return strings.HasSuffix(host, suffix)
	}
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

func normalizeStringList(items []string, normalize func(string) string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = normalize(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizeList(items []string) []string {
	return normalizeStringList(items, func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	})
}

func normalizeUpperList(items []string) []string {
	return normalizeStringList(items, func(s string) string {
		return strings.ToUpper(strings.TrimSpace(s))
	})
}

func normalizePathList(items []string) []string {
	return normalizeStringList(items, RequestPath)
}

func normalizeUint32List(items []uint32) []uint32 {
	if len(items) == 0 {
		return nil
	}
	out := append([]uint32(nil), items...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	writeAt := 0
	for _, value := range out {
		if writeAt != 0 && out[writeAt-1] == value {
			continue
		}
		out[writeAt] = value
		writeAt++
	}
	return out[:writeAt]
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}
