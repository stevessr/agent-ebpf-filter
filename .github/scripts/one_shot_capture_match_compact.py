from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing compact-match anchor in {path}: {old[:120]!r}")
    write(path, text.replace(old, new, 1))


profile = "backend/app/captureprofile/profile.go"
replace_once(profile,
'''\treturn matchIndexedSnapshot(snapshot, observation)
}

func ParseJSON''',
'''\treturn matchIndexedSnapshot(snapshot, observation, true)
}

// MatchCompact runs the same matcher as Match but omits diagnostic MatchedBy
// materialization. Production capture paths use this to avoid per-event
// diagnostic slice allocation; control-plane preview keeps using Match.
func (r *Registry) MatchCompact(observation Observation) (Match, bool) {
\tif r == nil {
\t\treturn Match{}, false
\t}
\tsnapshot := r.snapshot.Load()
\tif snapshot == nil || snapshot.index == nil {
\t\treturn Match{}, false
\t}
\treturn matchIndexedSnapshot(snapshot, observation, false)
}

func ParseJSON''')

index = "backend/app/captureprofile/profile_index.go"
replace_once(index,
'''func matchIndexedSnapshot(snapshot *profileSnapshot, observation Observation) (Match, bool) {''',
'''func matchIndexedSnapshot(snapshot *profileSnapshot, observation Observation, detailed bool) (Match, bool) {''')
replace_once(index,
'''\t\t\tcandidate, ok := matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared)''',
'''\t\t\tcandidate, ok := matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared, detailed)''')
replace_once(index,
'''\t\t\tif candidate, ok := matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared); ok {''',
'''\t\t\tif candidate, ok := matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared, true); ok {''')
replace_once(index,
'''func matchCompiledProfile(compiled compiledProfile, observation preparedObservation) (Match, bool) {''',
'''func matchCompiledProfile(compiled compiledProfile, observation preparedObservation, detailed bool) (Match, bool) {''')
replace_once(index,
'''\tmatchedBy := make([]string, 0, 12+len(profile.RequiredHeaders))
\tfields := []struct {''',
'''\tconfidence := uint32(score)
\tif confidence > 100 {
\t\tconfidence = 100
\t}
\tif !detailed {
\t\treturn Match{
\t\t\tProfileID: profile.ID, Vendor: profile.Vendor, Product: profile.Product, Operation: profile.Operation,
\t\t\tConfidence: confidence, Score: score,
\t\t}, true
\t}

\tmatchedBy := make([]string, 0, 12+len(profile.RequiredHeaders))
\tfields := []struct {''')
replace_once(index,
'''\tconfidence := uint32(score)
\tif confidence > 100 {
\t\tconfidence = 100
\t}
\treturn Match{''',
'''\treturn Match{''')

for path in ["backend/app/tls/api_fingerprint.go", "backend/app/events/events_network.go"]:
    replace_once(path, "captureprofile.Default.Match(captureprofile.Observation{", "captureprofile.Default.MatchCompact(captureprofile.Observation{")

bench = "backend/app/captureprofile/profile_bench_test.go"
text = read(bench)
if "BenchmarkRegistryMatchCompact512" not in text:
    text += r'''

func BenchmarkRegistryMatchCompact512(b *testing.B) {
    registry := benchmarkRegistry(512)
    observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42"}
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        if match, ok := registry.MatchCompact(observation); !ok || match.ProfileID != "target" {
            b.Fatal("target profile did not match")
        }
    }
}
'''
    write(bench, text)

test = "backend/app/captureprofile/profile_test.go"
text = read(test)
if "TestRegistryMatchCompactEquivalent" not in text:
    text += r'''

func TestRegistryMatchCompactEquivalent(t *testing.T) {
    registry := NewRegistry([]Profile{{
        ID: "compact", Vendor: "acme", Product: "jobs", Operation: "create",
        Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"}, Methods: []string{"POST"},
        HostSuffixes: []string{"api.example.test"}, PathPrefixes: []string{"/v1/jobs"}, MinScore: 90,
    }})
    observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Method: "POST", Host: "api.example.test", Path: "/v1/jobs/42"}
    detailed, ok := registry.Match(observation)
    if !ok || len(detailed.MatchedBy) == 0 {
        t.Fatalf("detailed match missing diagnostics: ok=%v match=%+v", ok, detailed)
    }
    compact, ok := registry.MatchCompact(observation)
    if !ok {
        t.Fatal("compact match failed")
    }
    if compact.ProfileID != detailed.ProfileID || compact.Score != detailed.Score || compact.Confidence != detailed.Confidence || compact.Vendor != detailed.Vendor || compact.Product != detailed.Product || compact.Operation != detailed.Operation {
        t.Fatalf("compact mismatch: detailed=%+v compact=%+v", detailed, compact)
    }
    if compact.MatchedBy != nil {
        t.Fatalf("compact match should omit diagnostics: %+v", compact.MatchedBy)
    }
}
'''
    write(test, text)
