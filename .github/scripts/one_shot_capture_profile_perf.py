from pathlib import Path
import re


def read(path):
    return Path(path).read_text()


def write(path, content):
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    write(path, text.replace(old, new, 1))


def regex_once(path, pattern, repl):
    text = read(path)
    updated, count = re.subn(pattern, repl, text, count=1, flags=re.S)
    if count != 1:
        raise SystemExit(f"regex anchor count={count} in {path}: {pattern[:120]!r}")
    write(path, updated)


profile = "backend/app/captureprofile/profile.go"
replace_once(profile,
'''type Observation struct {
\tSource      string
\tProtocol    string
\tDirection   string
\tMethod      string
\tHost        string
\tPath        string
\tHeaders     map[string]string
\tContentType string
}''',
'''type Observation struct {
\tSource      string
\tProtocol    string
\tDirection   string
\tMethod      string
\tHost        string
\tPath        string
\tHeaders     map[string]string
\tContentType string
\tTransport   string
\tFamily      string
\tRemoteIP    string
\tRemotePort  uint32
\tProcess     string
}''')

replace_once(profile,
'''\tRequiredHeaders []string `json:"required_headers,omitempty"`
\tContentTypes    []string `json:"content_types,omitempty"`
\tMinScore        int      `json:"min_score,omitempty"`''',
'''\tRequiredHeaders []string `json:"required_headers,omitempty"`
\tContentTypes    []string `json:"content_types,omitempty"`
\tTransports      []string `json:"transports,omitempty"`
\tFamilies        []string `json:"families,omitempty"`
\tRemotePorts     []uint32 `json:"remote_ports,omitempty"`
\tRemoteCIDRs     []string `json:"remote_cidrs,omitempty"`
\tProcesses       []string `json:"processes,omitempty"`
\tMinScore        int      `json:"min_score,omitempty"`''')

replace_once(profile,
'''type profileSnapshot struct {
\tprofiles []Profile
}''',
'''type profileSnapshot struct {
\tprofiles   []Profile
\tindex      *profileDispatchIndex
\tgeneration uint64
}''')

replace_once(profile,
'''type Registry struct {
\tsnapshot atomic.Pointer[profileSnapshot]
}''',
'''type Registry struct {
\tsnapshot   atomic.Pointer[profileSnapshot]
\tgeneration atomic.Uint64
}''')

replace_once(profile,
'''\tsort.SliceStable(validated, func(i, j int) bool { return validated[i].ID < validated[j].ID })
\tr.snapshot.Store(&profileSnapshot{profiles: validated})
\treturn nil''',
'''\tsort.SliceStable(validated, func(i, j int) bool { return validated[i].ID < validated[j].ID })
\tindex, err := compileProfileDispatch(validated)
\tif err != nil {
\t\treturn err
\t}
\tgeneration := r.generation.Add(1)
\tindex.stats.Generation = generation
\tr.snapshot.Store(&profileSnapshot{profiles: validated, index: index, generation: generation})
\treturn nil''')

regex_once(profile,
r'''func \(r \*Registry\) Match\(observation Observation\) \(Match, bool\) \{.*?\n\}\n\nfunc ParseJSON''',
'''func (r *Registry) Match(observation Observation) (Match, bool) {
\tif r == nil {
\t\treturn Match{}, false
\t}
\tsnapshot := r.snapshot.Load()
\tif snapshot == nil || snapshot.index == nil {
\t\treturn Match{}, false
\t}
\treturn matchIndexedSnapshot(snapshot, observation)
}

func ParseJSON''')

replace_once(profile,
'''\tprofile.RequiredHeaders = normalizeList(profile.RequiredHeaders)
\tprofile.ContentTypes = normalizeList(profile.ContentTypes)
\treturn profile''',
'''\tprofile.RequiredHeaders = normalizeList(profile.RequiredHeaders)
\tprofile.ContentTypes = normalizeList(profile.ContentTypes)
\tprofile.Transports = normalizeList(profile.Transports)
\tprofile.Families = normalizeList(profile.Families)
\tprofile.RemotePorts = normalizeUint32List(profile.RemotePorts)
\tprofile.RemoteCIDRs = normalizeList(profile.RemoteCIDRs)
\tprofile.Processes = normalizeList(profile.Processes)
\treturn profile''')

replace_once(profile,
'''\tobservation.ContentType = strings.ToLower(strings.TrimSpace(observation.ContentType))
\treturn observation''',
'''\tobservation.ContentType = strings.ToLower(strings.TrimSpace(observation.ContentType))
\tobservation.Transport = strings.ToLower(strings.TrimSpace(observation.Transport))
\tobservation.Family = strings.ToLower(strings.TrimSpace(observation.Family))
\tobservation.RemoteIP = strings.TrimSpace(observation.RemoteIP)
\tobservation.Process = strings.ToLower(strings.TrimSpace(observation.Process))
\treturn observation''')

replace_once(profile,
'''func normalizeList(items []string) []string {
\tout := make([]string, 0, len(items))
\tfor _, item := range items {
\t\titem = strings.ToLower(strings.TrimSpace(item))
\t\tif item != "" {
\t\t\tout = append(out, item)
\t\t}
\t}
\treturn out
}''',
'''func normalizeList(items []string) []string {
\tif len(items) == 0 {
\t\treturn nil
\t}
\tseen := make(map[string]struct{}, len(items))
\tout := make([]string, 0, len(items))
\tfor _, item := range items {
\t\titem = strings.ToLower(strings.TrimSpace(item))
\t\tif item == "" {
\t\t\tcontinue
\t\t}
\t\tif _, ok := seen[item]; ok {
\t\t\tcontinue
\t\t}
\t\tseen[item] = struct{}{}
\t\tout = append(out, item)
\t}
\tsort.Strings(out)
\treturn out
}''')

replace_once(profile,
'''func normalizeUpperList(items []string) []string {
\tout := make([]string, 0, len(items))
\tfor _, item := range items {
\t\titem = strings.ToUpper(strings.TrimSpace(item))
\t\tif item != "" {
\t\t\tout = append(out, item)
\t\t}
\t}
\treturn out
}''',
'''func normalizeUpperList(items []string) []string {
\tif len(items) == 0 {
\t\treturn nil
\t}
\tseen := make(map[string]struct{}, len(items))
\tout := make([]string, 0, len(items))
\tfor _, item := range items {
\t\titem = strings.ToUpper(strings.TrimSpace(item))
\t\tif item == "" {
\t\t\tcontinue
\t\t}
\t\tif _, ok := seen[item]; ok {
\t\t\tcontinue
\t\t}
\t\tseen[item] = struct{}{}
\t\tout = append(out, item)
\t}
\tsort.Strings(out)
\treturn out
}''')

replace_once(profile,
'''func normalizePathList(items []string) []string {
\tout := make([]string, 0, len(items))
\tfor _, item := range items {
\t\titem = RequestPath(item)
\t\tif item != "" {
\t\t\tout = append(out, item)
\t\t}
\t}
\treturn out
}''',
'''func normalizePathList(items []string) []string {
\tif len(items) == 0 {
\t\treturn nil
\t}
\tseen := make(map[string]struct{}, len(items))
\tout := make([]string, 0, len(items))
\tfor _, item := range items {
\t\titem = RequestPath(item)
\t\tif item == "" {
\t\t\tcontinue
\t\t}
\t\tif _, ok := seen[item]; ok {
\t\t\tcontinue
\t\t}
\t\tseen[item] = struct{}{}
\t\tout = append(out, item)
\t}
\tsort.Strings(out)
\treturn out
}

func normalizeUint32List(items []uint32) []uint32 {
\tif len(items) == 0 {
\t\treturn nil
\t}
\tout := append([]uint32(nil), items...)
\tsort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
\twriteAt := 0
\tfor _, value := range out {
\t\tif writeAt != 0 && out[writeAt-1] == value {
\t\t\tcontinue
\t\t}
\t\tout[writeAt] = value
\t\twriteAt++
\t}
\treturn out[:writeAt]
}''')

write("backend/app/captureprofile/profile_index.go", r'''package captureprofile

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
    profile      Profile
    remoteCIDRs  []netip.Prefix
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

func matchIndexedSnapshot(snapshot *profileSnapshot, observation Observation) (Match, bool) {
    prepared := prepareObservation(observation)
    keys, count := dispatchCandidateKeys(prepared)
    best := Match{}
    matched := false
    for i := 0; i < count; i++ {
        for _, profileIndex := range snapshot.index.buckets[keys[i]] {
            candidate, ok := matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared)
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
            if candidate, ok := matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared); ok {
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

func matchCompiledProfile(compiled compiledProfile, observation preparedObservation) (Match, bool) {
    profile := compiled.profile
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
        if !containsExact(profile.Sources, observation.Source) { return Match{}, false }
        score += 3; matched |= matchedSource
    }
    if len(profile.Protocols) != 0 {
        if !containsExact(profile.Protocols, observation.Protocol) { return Match{}, false }
        score += 5; matched |= matchedProtocol
    }
    if len(profile.Directions) != 0 {
        if !containsExact(profile.Directions, observation.Direction) { return Match{}, false }
        score += 3; matched |= matchedDirection
    }
    if len(profile.Methods) != 0 {
        if !containsExact(profile.Methods, observation.Method) { return Match{}, false }
        score += 5; matched |= matchedMethod
    }
    if len(profile.Transports) != 0 {
        if !containsExact(profile.Transports, observation.Transport) { return Match{}, false }
        score += 3; matched |= matchedTransport
    }
    if len(profile.Families) != 0 {
        if !containsExact(profile.Families, observation.Family) { return Match{}, false }
        score += 3; matched |= matchedFamily
    }
    if len(profile.RemotePorts) != 0 {
        if !containsPort(profile.RemotePorts, observation.RemotePort) { return Match{}, false }
        score += 8; matched |= matchedRemotePort
    }
    if len(compiled.remoteCIDRs) != 0 {
        if !observation.remoteAddr.IsValid() { return Match{}, false }
        cidrMatch := false
        for _, prefix := range compiled.remoteCIDRs {
            if prefix.Contains(observation.remoteAddr) { cidrMatch = true; break }
        }
        if !cidrMatch { return Match{}, false }
        score += 12; matched |= matchedRemoteCIDR
    }
    if len(profile.Processes) != 0 {
        if !containsExact(profile.Processes, observation.Process) { return Match{}, false }
        score += 5; matched |= matchedProcess
    }

    hostMatched := false
    if observation.Host != "" {
        for _, suffix := range profile.HostSuffixes {
            if hostSuffixMatch(observation.Host, suffix) { hostMatched = true; break }
        }
        if !hostMatched {
            for _, fragment := range profile.HostContains {
                if strings.Contains(observation.Host, fragment) { hostMatched = true; break }
            }
        }
    }
    if len(profile.HostSuffixes)+len(profile.HostContains) != 0 && !hostMatched { return Match{}, false }
    if hostMatched { score += 60; matched |= matchedHost }

    pathMatched := false
    if observation.Path != "" {
        for _, prefix := range profile.PathPrefixes {
            if strings.HasPrefix(observation.Path, prefix) { pathMatched = true; break }
        }
        if !pathMatched {
            for _, fragment := range profile.PathContains {
                if strings.Contains(observation.Path, fragment) { pathMatched = true; break }
            }
        }
    }
    if len(profile.PathPrefixes)+len(profile.PathContains) != 0 && !pathMatched { return Match{}, false }
    if pathMatched { score += 30; matched |= matchedPath }

    for _, header := range profile.RequiredHeaders {
        if !hasHeader(observation.Headers, header) { return Match{}, false }
        score += 5
    }
    if len(profile.ContentTypes) != 0 {
        contentTypeMatch := false
        for _, contentType := range profile.ContentTypes {
            if strings.Contains(observation.ContentType, contentType) { contentTypeMatch = true; break }
        }
        if !contentTypeMatch { return Match{}, false }
        score += 5; matched |= matchedContentType
    }
    if score < profile.MinScore { return Match{}, false }

    matchedBy := make([]string, 0, 12+len(profile.RequiredHeaders))
    fields := []struct{ bit uint32; label string }{
        {matchedSource, "source"}, {matchedProtocol, "protocol"}, {matchedDirection, "direction"},
        {matchedMethod, "method"}, {matchedTransport, "transport"}, {matchedFamily, "family"},
        {matchedRemotePort, "remote-port"}, {matchedRemoteCIDR, "remote-cidr"}, {matchedProcess, "process"},
        {matchedHost, "host"}, {matchedPath, "path"}, {matchedContentType, "content-type"},
    }
    for _, field := range fields {
        if matched&field.bit != 0 { matchedBy = append(matchedBy, field.label) }
    }
    for _, header := range profile.RequiredHeaders {
        matchedBy = append(matchedBy, "header:"+header)
    }
    confidence := uint32(score)
    if confidence > 100 { confidence = 100 }
    return Match{
        ProfileID: profile.ID, Vendor: profile.Vendor, Product: profile.Product, Operation: profile.Operation,
        Confidence: confidence, Score: score, MatchedBy: matchedBy,
    }, true
}
''')

# Extend tests and add benchmarks.
test_path = "backend/app/captureprofile/profile_test.go"
text = read(test_path)
text += r'''

func TestRegistryIndexedSelectorsAndMatchAll(t *testing.T) {
    registry := NewRegistry([]Profile{
        {ID: "generic", Vendor: "generic", Protocols: []string{"http1"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v1"}, MinScore: 35},
        {ID: "scoped", Vendor: "scoped", Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"}, Directions: []string{"outgoing"}, Methods: []string{"POST"}, Transports: []string{"tcp"}, Families: []string{"ipv4"}, RemotePorts: []uint32{8443}, RemoteCIDRs: []string{"10.0.0.0/8"}, Processes: []string{"curl"}, PathPrefixes: []string{"/v1"}, MinScore: 70},
    })
    observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", RemoteIP: "10.1.2.3", RemotePort: 8443, Process: "curl", Path: "/v1/jobs"}
    match, ok := registry.Match(observation)
    if !ok || match.ProfileID != "scoped" {
        t.Fatalf("indexed scoped match failed: ok=%v match=%+v", ok, match)
    }
    matches := registry.MatchAll(observation)
    if len(matches) != 2 || matches[0].ProfileID != "scoped" || matches[1].ProfileID != "generic" {
        t.Fatalf("unexpected MatchAll: %+v", matches)
    }
    stats := registry.Stats()
    if stats.Profiles != 2 || stats.DispatchBuckets == 0 || stats.Generation == 0 {
        t.Fatalf("unexpected registry stats: %+v", stats)
    }
}

func TestRegistryRejectsInvalidNetworkSelectors(t *testing.T) {
    registry := NewRegistry(nil)
    if err := registry.Replace([]Profile{{ID: "bad-port", Vendor: "x", RemotePorts: []uint32{70000}}}); err == nil {
        t.Fatal("expected invalid remote port to be rejected")
    }
    if err := registry.Replace([]Profile{{ID: "bad-cidr", Vendor: "x", RemoteCIDRs: []string{"not-a-cidr"}}}); err == nil {
        t.Fatal("expected invalid CIDR to be rejected")
    }
}
'''
write(test_path, text)

write("backend/app/captureprofile/profile_bench_test.go", r'''package captureprofile

import (
    "fmt"
    "testing"
)

func benchmarkRegistry(size int) *Registry {
    profiles := make([]Profile, 0, size)
    protocols := []string{"http1", "http2", "grpc"}
    methods := []string{"GET", "POST", "PUT", "PATCH"}
    sources := []string{"kernel_socket_prefix", "tls_plaintext"}
    for i := 0; i < size; i++ {
        profiles = append(profiles, Profile{
            ID: fmt.Sprintf("bench.%04d", i), Vendor: "bench",
            Sources: []string{sources[i%len(sources)]}, Protocols: []string{protocols[i%len(protocols)]},
            Directions: []string{"outgoing"}, Methods: []string{methods[i%len(methods)]},
            Transports: []string{"tcp"}, HostSuffixes: []string{fmt.Sprintf("api-%d.example.test", i)},
            PathPrefixes: []string{"/v1"}, MinScore: 90,
        })
    }
    profiles = append(profiles, Profile{
        ID: "target", Vendor: "target", Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"},
        Directions: []string{"outgoing"}, Methods: []string{"POST"}, Transports: []string{"tcp"},
        HostSuffixes: []string{"target.example.test"}, PathPrefixes: []string{"/v1/jobs"}, MinScore: 90,
    })
    return NewRegistry(profiles)
}

func BenchmarkRegistryMatchIndexed512(b *testing.B) {
    registry := benchmarkRegistry(512)
    observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42"}
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        if match, ok := registry.Match(observation); !ok || match.ProfileID != "target" {
            b.Fatal("target profile did not match")
        }
    }
}
''')

# Control-plane capabilities and matcher diagnostics.
control = "backend/app/captureprofile_control.go"
replace_once(control,
'''\tMethods         []string `json:"methods"`
\tPrivacyBoundary string   `json:"privacyBoundary"`''',
'''\tMethods         []string `json:"methods"`
\tTransports      []string `json:"transports"`
\tFamilies        []string `json:"families"`
\tPrivacyBoundary string   `json:"privacyBoundary"`''')
replace_once(control,
'''\tCapabilities      captureProfileCapabilities `json:"capabilities"`
}''',
'''\tCapabilities      captureProfileCapabilities `json:"capabilities"`
\tMatcher           captureprofile.RegistryStats `json:"matcher"`
}''')
replace_once(control,
'''\t\tMethods:         []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT"},
\t\tPrivacyBoundary:''',
'''\t\tMethods:         []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT"},
\t\tTransports:      []string{"tcp", "udp"},
\t\tFamilies:        []string{"ipv4", "ipv6"},
\t\tPrivacyBoundary:''')
replace_once(control,
'''\t\tUpdatedAt:         updatedAt,
\t\tCapabilities:      captureProfileCapabilitiesValue(),''',
'''\t\tUpdatedAt:         updatedAt,
\t\tCapabilities:      captureProfileCapabilitiesValue(),
\t\tMatcher:           captureprofile.Default.Stats(),''')

# TLS observations now expose process + transport to generic selectors.
tls_path = "backend/app/tls/api_fingerprint.go"
replace_once(tls_path,
'''\t\tHeaders:     event.Headers,
\t\tContentType: event.ContentType,
\t})''',
'''\t\tHeaders:     event.Headers,
\t\tContentType: event.ContentType,
\t\tTransport:   "tcp",
\t\tProcess:     event.Comm,
\t})''')

# Preserve socket type through the existing ABI-reserved word without growing the event.
common = "backend/ebpf/agent_tracker_common.h"
replace_once(common,
'''    meta->capture_flags |= socket->provenance_flags;
    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));''',
'''    meta->capture_flags |= socket->provenance_flags;
    meta->capture_reserved = socket->sock_type;
    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));''')
replace_once(common,
'''    e->kernel_capture_flags = meta->capture_flags;
    __builtin_memcpy(e->net_addr, meta->net_addr, 16);''',
'''    e->kernel_capture_flags = meta->capture_flags;
    e->kernel_capture_reserved = meta->capture_reserved;
    __builtin_memcpy(e->net_addr, meta->net_addr, 16);''')

regex_once(common,
r'''static __always_inline u32 capture_http1_request_line\(char \*dst, const void \*user_buf, u32 len\) \{.*?static __always_inline u32 capture_http1_response_line\(char \*dst, const void \*user_buf, u32 len\) \{.*?\n\}\n\n// Convenience inline''',
r'''#define HTTP1_START_NONE 0
#define HTTP1_START_REQUEST 1
#define HTTP1_START_RESPONSE 2

static __always_inline int looks_like_http1_response(const char *head, u32 len) {
    if (!head || len < 8) return 0;
    return head[0] == 'H' && head[1] == 'T' && head[2] == 'T' && head[3] == 'P' &&
           head[4] == '/' && head[5] == '1' && head[6] == '.';
}

// Classify and copy an HTTP/1 start-line with a single 8-byte probe. The old
// request-then-response path probed non-HTTP buffers twice. This helper keeps
// identical privacy semantics while halving the head probes on the hot path.
static __always_inline u32 capture_http1_start_line(char *dst, const void *user_buf, u32 len, u32 *kind) {
    if (kind) *kind = HTTP1_START_NONE;
    if (!dst || !user_buf || len < 4 || !kind) return 0;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;

    int request = looks_like_http1_method(head, head_len);
    int response = !request && looks_like_http1_response(head, head_len);
    if (!request && !response) return 0;
    *kind = request ? HTTP1_START_REQUEST : HTTP1_START_RESPONSE;

    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) {
        *kind = HTTP1_START_NONE;
        return 0;
    }
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        char c = dst[i];
        if (c == '\r' || c == '\n' || (request && (c == '?' || c == '#'))) {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}

// Convenience inline''')

# Collapse both syscall call sites to the single classifier.
syscalls = "backend/ebpf/agent_tracker_syscalls.h"
regex_once(syscalls,
r'''u32 captured = capture_http1_request_line\(pd->extra4, \(const void \*\)ctx->args\[1\], data_len\);\n        if \(captured > 0\) \{\n            meta.type = TYPE_SOCKET_HTTP;\n            meta.extra2 = captured;\n            meta.capture_flags \|= SOCKET_CAPTURE_HTTP1_REQUEST_LINE \| SOCKET_CAPTURE_OUTGOING;\n            __builtin_memcpy\(pd->path, "socket http", 12\);\n        \} else \{\n            captured = capture_http1_response_line\(pd->extra4, \(const void \*\)ctx->args\[1\], data_len\);\n            if \(captured > 0\) \{\n                meta.type = TYPE_SOCKET_HTTP;\n                meta.extra2 = captured;\n                meta.capture_flags \|= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE \| SOCKET_CAPTURE_OUTGOING;\n                __builtin_memcpy\(pd->path, "socket http", 12\);\n            \}\n        \}''',
'''u32 start_kind = HTTP1_START_NONE;
        u32 captured = capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], data_len, &start_kind);
        if (captured > 0) {
            meta.type = TYPE_SOCKET_HTTP;
            meta.extra2 = captured;
            meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
            if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
            else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
            __builtin_memcpy(pd->path, "socket http", 12);
        }''')

regex_once(syscalls,
r'''captured = capture_http1_request_line\(pd->extra4, \(const void \*\)meta.addr_ptr, actual\);\n        if \(captured > 0\) \{\n            meta.capture_flags \|= SOCKET_CAPTURE_HTTP1_REQUEST_LINE \| SOCKET_CAPTURE_INCOMING;\n        \} else \{\n            captured = capture_http1_response_line\(pd->extra4, \(const void \*\)meta.addr_ptr, actual\);\n            if \(captured > 0\) meta.capture_flags \|= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE \| SOCKET_CAPTURE_INCOMING;\n        \}''',
'''u32 start_kind = HTTP1_START_NONE;
        captured = capture_http1_start_line(pd->extra4, (const void *)meta.addr_ptr, actual, &start_kind);
        if (captured > 0) {
            meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
            if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
            else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
        }''')

# Make the existing ABI-reserved word usable in Go as socket type metadata.
core = "backend/core/types.go"
replace_once(core,
'''\tKernelCaptureFlags                     uint32
\t_                                      [4]byte // append-only capture ABI alignment''',
'''\tKernelCaptureFlags                     uint32
\tKernelSocketType                        uint32 // reuses append-only capture ABI word; 1=stream, 2=datagram, 3=raw''')

# Kernel fingerprint observations can now use family/IP/port/process/transport.
events = "backend/app/events/events_network.go"
replace_once(events,
'''\t\tif ok && startLine.Kind == "request" {
\t\t\tout.HttpMethod = startLine.Method''',
'''\t\ttransport := ""
\t\tswitch event.KernelSocketType {
\t\tcase 1:
\t\t\ttransport = "tcp"
\t\t\tout.SockType = "SOCK_STREAM"
\t\tcase 2:
\t\t\ttransport = "udp"
\t\t\tout.SockType = "SOCK_DGRAM"
\t\tcase 3:
\t\t\tout.SockType = "SOCK_RAW"
\t\t}
\t\tremoteIP := ""
\t\tif addr := NetworkIP(event.NetFamily, event.NetAddr); addr != nil {
\t\t\tremoteIP = addr.String()
\t\t}
\t\tif ok && startLine.Kind == "request" {
\t\t\tout.HttpMethod = startLine.Method''')
replace_once(events,
'''\t\t\tif match, matched := captureprofile.Default.Match(captureprofile.Observation{
\t\t\t\tSource: "kernel_socket_prefix", Protocol: "http1", Direction: direction,
\t\t\t\tMethod: startLine.Method, Path: startLine.Path,
\t\t\t}); matched {''',
'''\t\t\tif match, matched := captureprofile.Default.Match(captureprofile.Observation{
\t\t\t\tSource: "kernel_socket_prefix", Protocol: "http1", Direction: direction,
\t\t\t\tMethod: startLine.Method, Path: startLine.Path, Transport: transport,
\t\t\t\tFamily: NetworkFamilyLabel(event.NetFamily), RemoteIP: remoteIP, RemotePort: event.NetPort, Process: comm,
\t\t\t}); matched {''')

# Front-end selector model and preview.
front = "frontend/src/views/network/SocketCaptureProfiles.vue"
replace_once(front,
'''  required_headers?: string[];
  content_types?: string[];
  min_score?: number;''',
'''  required_headers?: string[];
  content_types?: string[];
  transports?: string[];
  families?: string[];
  remote_ports?: number[];
  remote_cidrs?: string[];
  processes?: string[];
  min_score?: number;''')
replace_once(front,
'''    methods: string[];
    privacyBoundary: string;''',
'''    methods: string[];
    transports: string[];
    families: string[];
    privacyBoundary: string;''')
replace_once(front,
'''  capabilities: {
    sources: string[];''',
'''  matcher: {
    generation: number;
    profiles: number;
    dispatchBuckets: number;
    dispatchEntries: number;
    maxBucket: number;
  };
  capabilities: {
    sources: string[];''')
replace_once(front,
'''  content_types: [],
  min_score: 60,''',
'''  content_types: [],
  transports: [],
  families: [],
  remote_ports: [],
  remote_cidrs: [],
  processes: [],
  min_score: 60,''')
replace_once(front,
'''const draft = reactive<CaptureProfile>(emptyProfile());''',
'''const draft = reactive<CaptureProfile>(emptyProfile());
const remotePortsText = computed({
  get: () => (draft.remote_ports || []).join(", "),
  set: (value: string) => {
    draft.remote_ports = value
      .split(/[\\s,]+/)
      .map((item) => Number(item))
      .filter((port) => Number.isInteger(port) && port > 0 && port <= 65535);
  },
});''')
replace_once(front,
'''    methods: flow.httpMethod ? [flow.httpMethod.toUpperCase()] : [],
    host_suffixes: host ? [host] : [],''',
'''    methods: flow.httpMethod ? [flow.httpMethod.toUpperCase()] : [],
    transports: [(flow.transport || flow.protocol || "tcp").toLowerCase()],
    processes: (flow.processComms || []).slice(0, 1).map((value) => value.toLowerCase()),
    host_suffixes: host ? [host] : [],''')
replace_once(front,
'''    Headers: {},
    ContentType: "",
  }));''',
'''    Headers: {},
    ContentType: "",
    Transport: (flow.transport || flow.protocol || "").toLowerCase(),
    Family: flow.dstIp ? (flow.dstIp.includes(":") ? "ipv6" : "ipv4") : "",
    RemoteIP: flow.dstIp || "",
    RemotePort: flow.dstPort || 0,
    Process: (flow.processComms?.[0] || "").toLowerCase(),
  }));''')
replace_once(front,
'''          <div class="meta-line">{{ state?.builtinCount || 0 }} built-in + {{ state?.customCount || 0 }} custom</div>''',
'''          <div class="meta-line">{{ state?.builtinCount || 0 }} built-in + {{ state?.customCount || 0 }} custom · {{ state?.matcher.dispatchBuckets || 0 }} index buckets · max {{ state?.matcher.maxBucket || 0 }}/bucket</div>''')
replace_once(front,
'''            <div v-if="record.methods?.length">method: {{ record.methods.join(', ') }}</div>
            <span v-if="!record.host_suffixes?.length && !record.path_prefixes?.length && !record.methods?.length" class="meta-line">score-based generic rule</span>''',
'''            <div v-if="record.methods?.length">method: {{ record.methods.join(', ') }}</div>
            <div v-if="record.remote_ports?.length">port: {{ record.remote_ports.join(', ') }}</div>
            <div v-if="record.processes?.length">process: {{ record.processes.join(', ') }}</div>
            <span v-if="!record.host_suffixes?.length && !record.path_prefixes?.length && !record.methods?.length && !record.remote_ports?.length && !record.processes?.length" class="meta-line">score-based generic rule</span>''')
replace_once(front,
'''        <a-form-item label="HTTP methods">
          <a-select v-model:value="draft.methods" mode="multiple" :options="(state?.capabilities.methods || []).map(value => ({ value, label: value }))" @change="previewDraft" />
        </a-form-item>

        <a-divider orientation="left">API fingerprint</a-divider>''',
'''        <a-form-item label="HTTP methods">
          <a-select v-model:value="draft.methods" mode="multiple" :options="(state?.capabilities.methods || []).map(value => ({ value, label: value }))" @change="previewDraft" />
        </a-form-item>

        <a-divider orientation="left">Socket / process scope</a-divider>
        <a-row :gutter="12">
          <a-col :xs="24" :md="12">
            <a-form-item label="Transports">
              <a-select v-model:value="draft.transports" mode="multiple" :options="(state?.capabilities.transports || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :md="12">
            <a-form-item label="Address families">
              <a-select v-model:value="draft.families" mode="multiple" :options="(state?.capabilities.families || []).map(value => ({ value, label: value }))" @change="previewDraft" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="Remote ports">
          <a-input v-model:value="remotePortsText" placeholder="443, 8443" @blur="previewDraft" />
        </a-form-item>
        <a-form-item label="Remote CIDRs">
          <a-select v-model:value="draft.remote_cidrs" mode="tags" placeholder="10.0.0.0/8" @change="previewDraft" />
        </a-form-item>
        <a-form-item label="Processes">
          <a-select v-model:value="draft.processes" mode="tags" placeholder="curl" @change="previewDraft" />
        </a-form-item>

        <a-divider orientation="left">API fingerprint</a-divider>''')

# Documentation: append performance/generalization notes.
doc = "docs/backend/generic-api-capture.md"
text = read(doc)
text += r'''

## Indexed matcher and socket-scope selectors

The profile registry compiles immutable rules into dispatch buckets keyed by the
high-frequency exact selectors `source`, `protocol`, `direction`, `method`, and
`transport`. A lookup probes at most 32 exact/wildcard bucket combinations and
then runs host/path/header/CIDR checks only for those candidates. Profile reload
still uses atomic snapshot publication, so capture readers take no mutex.

Profiles may additionally constrain `transports`, `families`, `remote_ports`,
`remote_cidrs`, and `processes`. Kernel socket-prefix events feed family,
endpoint port/IP, process name, and the socket type carried in the existing
append-only ABI word; TLS plaintext events feed process and TCP transport. A
selector whose metadata is unavailable fails closed for that profile rather
than silently matching a broader scope.

HTTP/1 socket start-line classification now performs one bounded 8-byte userspace
probe to distinguish request/response/non-HTTP before copying a start-line. This
replaces the previous request probe followed by a second response probe on the
same syscall while preserving the same query/fragment truncation boundary.
'''
write(doc, text)
