package ml

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTrainingDataStoreNormalizesAndOwnsSampleMetadata(t *testing.T) {
	args := []string{"owned", strings.Repeat("\x01", 900), strings.Repeat("界", 300) + "\xff"}
	for len(args) < trainingSampleMaxArgs+20 {
		args = append(args, "extra")
	}
	var features [FeatureDim]float64
	features[0] = math.NaN()
	features[1] = math.Inf(1)
	features[2] = math.Inf(-1)

	store := NewTrainingDataStore(2)
	store.Add(TrainingSample{
		Features:     features,
		Label:        99,
		CommandLine:  strings.Repeat("命令", trainingSampleMaxCommandLineBytes) + "\xff",
		Comm:         strings.Repeat("c", trainingSampleMaxCommBytes+100) + "\xff",
		Args:         args,
		Category:     strings.Repeat("类别", trainingSampleMaxCategoryBytes) + "\xff",
		AnomalyScore: math.NaN(),
		Timestamp:    time.Date(2500, 1, 1, 0, 0, 0, 0, time.FixedZone("future", 3600)),
		UserLabel:    strings.Repeat("label", trainingSampleMaxUserLabelBytes) + "\xff",
	})
	args[0] = "caller-mutated"

	samples := store.AllSamples()
	if len(samples) != 1 {
		t.Fatalf("sample count = %d, want 1", len(samples))
	}
	sample := samples[0]
	if sample.Args[0] != "owned" {
		t.Fatalf("stored args alias caller storage: %#v", sample.Args)
	}
	if sample.Label != -1 {
		t.Fatalf("normalized label = %d, want -1", sample.Label)
	}
	if sample.AnomalyScore != 0 {
		t.Fatalf("normalized anomaly score = %v, want 0", sample.AnomalyScore)
	}
	for index := 0; index < 3; index++ {
		if sample.Features[index] != 0 {
			t.Fatalf("normalized feature[%d] = %v, want 0", index, sample.Features[index])
		}
	}
	if sample.Timestamp.After(trainingSampleMaxTimestamp) || sample.Timestamp.Before(trainingSampleMinTimestamp) {
		t.Fatalf("timestamp %s is outside persistable range", sample.Timestamp)
	}
	assertBoundedTrainingString(t, "commandLine", sample.CommandLine, trainingSampleMaxCommandLineBytes)
	assertBoundedTrainingString(t, "comm", sample.Comm, trainingSampleMaxCommBytes)
	assertBoundedTrainingString(t, "category", sample.Category, trainingSampleMaxCategoryBytes)
	assertBoundedTrainingString(t, "userLabel", sample.UserLabel, trainingSampleMaxUserLabelBytes)
	if len(sample.Args) > trainingSampleMaxArgs {
		t.Fatalf("args count = %d, want <= %d", len(sample.Args), trainingSampleMaxArgs)
	}
	totalArgsBytes := 0
	for index, arg := range sample.Args {
		assertBoundedTrainingString(t, "arg", arg, trainingSampleMaxArgBytes)
		totalArgsBytes += len(arg)
		if index >= trainingSampleMaxArgs {
			t.Fatalf("unexpected arg index %d", index)
		}
	}
	if totalArgsBytes > trainingSampleMaxArgsBytes {
		t.Fatalf("args bytes = %d, want <= %d", totalArgsBytes, trainingSampleMaxArgsBytes)
	}
	encodedArgs, err := json.Marshal(sample.Args)
	if err != nil {
		t.Fatalf("marshal normalized args: %v", err)
	}
	if len(encodedArgs) > trainingSampleMaxArgsJSONBytes {
		t.Fatalf("encoded args bytes = %d, want <= %d", len(encodedArgs), trainingSampleMaxArgsJSONBytes)
	}
}

func TestTrainingDataStoreSnapshotsDoNotAliasStoredArgs(t *testing.T) {
	store := NewTrainingDataStore(4)
	store.Add(TrainingSample{
		Label:     1,
		Comm:      "echo",
		Args:      []string{"original"},
		Timestamp: time.Unix(1700000000, 0),
		UserLabel: "manual",
	})

	mutateAndAssertOwned := func(name string, samples []TrainingSample) {
		t.Helper()
		if len(samples) != 1 || len(samples[0].Args) != 1 {
			t.Fatalf("%s returned %#v", name, samples)
		}
		samples[0].Args[0] = "mutated"
		if got := store.AllSamples()[0].Args[0]; got != "original" {
			t.Fatalf("%s aliases stored args: got %q", name, got)
		}
	}
	mutateAndAssertOwned("AllSamples", store.AllSamples())
	mutateAndAssertOwned("LabeledSamples", store.LabeledSamples())

	indexedSnapshots := []struct {
		name    string
		samples []IndexedTrainingSample
	}{
		{name: "AllSamplesWithIndex", samples: store.AllSamplesWithIndex()},
		{name: "BoundedSamplesWithIndex", samples: store.BoundedSamplesWithIndex(1, false)},
		{name: "ExactMatches", samples: store.ExactMatches("echo", []string{"original"})},
	}
	for _, snapshot := range indexedSnapshots {
		if len(snapshot.samples) != 1 || len(snapshot.samples[0].Sample.Args) != 1 {
			t.Fatalf("%s returned %#v", snapshot.name, snapshot.samples)
		}
		snapshot.samples[0].Sample.Args[0] = "mutated"
		if got := store.AllSamples()[0].Args[0]; got != "original" {
			t.Fatalf("%s aliases stored args: got %q", snapshot.name, got)
		}
	}
}

func TestNormalizeTrainingStoreCapacity(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: -1, want: 1},
		{input: 0, want: 1},
		{input: 1, want: 1},
		{input: 42, want: 42},
		{input: trainingStoreMaxSamples + 1, want: trainingStoreMaxSamples},
	}
	for _, test := range tests {
		if got := NormalizeTrainingStoreCapacity(test.input); got != test.want {
			t.Errorf("NormalizeTrainingStoreCapacity(%d) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestTrainingDataStoreLoadRejectsCorruptionWithoutMutation(t *testing.T) {
	store := NewTrainingDataStore(4)
	store.dataDir = t.TempDir()
	store.persistPath = filepath.Join(store.dataDir, "ml_training_data.bin")
	store.Add(TrainingSample{Comm: "keep", Args: []string{"me"}, Timestamp: time.Unix(1700000000, 0)})

	corrupt := append([]byte(trainingStoreMagic), 1, 0, 0, 0)
	if err := os.WriteFile(store.persistPath, corrupt, 0o600); err != nil {
		t.Fatalf("write corrupt store: %v", err)
	}
	if err := store.LoadFromDisk(); err == nil {
		t.Fatal("LoadFromDisk() accepted a truncated record")
	}
	assertSingleTrainingCommand(t, store, "keep")

	var excessive bytes.Buffer
	excessive.WriteString(trainingStoreMagic)
	if err := binary.Write(&excessive, binary.LittleEndian, uint32(trainingStoreMaxSamples+1)); err != nil {
		t.Fatalf("encode excessive count: %v", err)
	}
	if err := os.WriteFile(store.persistPath, excessive.Bytes(), 0o600); err != nil {
		t.Fatalf("write excessive-count store: %v", err)
	}
	if err := store.LoadFromDisk(); err == nil {
		t.Fatal("LoadFromDisk() accepted an excessive record count")
	}
	assertSingleTrainingCommand(t, store, "keep")

	trailing := append([]byte(trainingStoreMagic), 0, 0, 0, 0, 1)
	if err := os.WriteFile(store.persistPath, trailing, 0o600); err != nil {
		t.Fatalf("write trailing-data store: %v", err)
	}
	if err := store.LoadFromDisk(); err == nil {
		t.Fatal("LoadFromDisk() accepted trailing data")
	}
	assertSingleTrainingCommand(t, store, "keep")
}

func TestTrainingDataStoreLoadValidatesRecordsBeyondCapacity(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ml_training_data.bin")
	samples := []TrainingSample{
		{Comm: "first", Timestamp: time.Unix(1700000000, 0)},
		{Comm: "second", Timestamp: time.Unix(1700000001, 0)},
	}
	var encoded bytes.Buffer
	if err := writeTrainingStore(&encoded, samples, len(samples)); err != nil {
		t.Fatalf("writeTrainingStore() error = %v", err)
	}
	payload := encoded.Bytes()
	if err := os.WriteFile(path, payload[:len(payload)-1], 0o600); err != nil {
		t.Fatalf("write truncated store: %v", err)
	}
	if _, err := readTrainingStoreFile(path, 1); err == nil {
		t.Fatal("readTrainingStoreFile() ignored corruption after retained capacity")
	}
}

func TestTrainingDataStoreLoadRejectsOversizedFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ml_training_data.bin")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create sparse store: %v", err)
	}
	if err := file.Truncate(trainingStoreMaxFileBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate sparse store: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close sparse store: %v", err)
	}
	if _, err := readTrainingStoreFile(path, 4); !errors.Is(err, errTrainingStoreTooLarge) {
		t.Fatalf("readTrainingStoreFile() error = %v, want errTrainingStoreTooLarge", err)
	}
}

func TestTrainingDataStoreMissingFilePreservesSamples(t *testing.T) {
	store := NewTrainingDataStore(2)
	store.dataDir = t.TempDir()
	store.persistPath = filepath.Join(store.dataDir, "missing.bin")
	store.Add(TrainingSample{Comm: "keep", Timestamp: time.Unix(1700000000, 0)})
	if err := store.LoadFromDisk(); err != nil {
		t.Fatalf("load missing store: %v", err)
	}
	assertSingleTrainingCommand(t, store, "keep")
}

func TestTrainingDataStoreLegacyFormatCompatibility(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "ml_training_data.bin")
	var encoded bytes.Buffer
	if err := binary.Write(&encoded, binary.LittleEndian, uint32(1)); err != nil {
		t.Fatalf("write count: %v", err)
	}
	var fixed [20]byte
	binary.LittleEndian.PutUint64(fixed[0:8], uint64(time.Unix(1700000000, 0).UnixNano()))
	binary.LittleEndian.PutUint32(fixed[8:12], uint32(1))
	binary.LittleEndian.PutUint64(fixed[12:20], math.Float64bits(0.75))
	encoded.Write(fixed[:])
	writeTestTrainingWirePayload(t, &encoded, []byte("rm"))
	writeTestTrainingWirePayload(t, &encoded, []byte("[-rf /tmp/demo]"))
	var features [FeatureDim * 8]byte
	binary.LittleEndian.PutUint64(features[0:8], math.Float64bits(0.5))
	encoded.Write(features[:])
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatalf("write legacy store: %v", err)
	}

	samples, err := readTrainingStoreFile(path, 4)
	if err != nil {
		t.Fatalf("read legacy store: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("legacy sample count = %d, want 1", len(samples))
	}
	sample := samples[0]
	if sample.Comm != "rm" || len(sample.Args) != 2 || sample.Args[0] != "-rf" || sample.Args[1] != "/tmp/demo" {
		t.Fatalf("legacy command = %q %#v", sample.Comm, sample.Args)
	}
	if sample.CommandLine != "rm -rf /tmp/demo" || sample.UserLabel != "loaded" || sample.Features[0] != 0.5 {
		t.Fatalf("legacy sample = %#v", sample)
	}
}

func TestTrainingDataStoreRejectsSymlinkAndHardlinkTargets(t *testing.T) {
	directory := t.TempDir()
	victimPath := filepath.Join(directory, "victim.bin")
	if err := os.WriteFile(victimPath, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	symlinkPath := filepath.Join(directory, "ml_training_data.bin")
	if err := os.Symlink(victimPath, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := readTrainingStoreFile(symlinkPath, 2); err == nil {
		t.Fatal("readTrainingStoreFile() followed a symlink")
	}
	store := NewTrainingDataStore(2)
	store.dataDir = directory
	store.persistPath = symlinkPath
	store.Add(TrainingSample{Comm: "echo", Timestamp: time.Unix(1700000000, 0)})
	if err := store.Flush(); err == nil {
		t.Fatal("Flush() replaced an unsafe symlink destination")
	}
	victim, err := os.ReadFile(victimPath)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(victim) != "unchanged" {
		t.Fatalf("victim content = %q", victim)
	}

	if err := os.Remove(symlinkPath); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	hardlinkPath := filepath.Join(directory, "hardlink.bin")
	if err := os.Link(victimPath, hardlinkPath); err != nil {
		t.Fatalf("create hardlink: %v", err)
	}
	if _, err := readTrainingStoreFile(hardlinkPath, 2); err == nil {
		t.Fatal("readTrainingStoreFile() accepted a multiply-linked file")
	}
}

func TestTrainingDataStoreFlushUsesPrivateAtomicFile(t *testing.T) {
	store := NewTrainingDataStore(2)
	store.dataDir = t.TempDir()
	store.persistPath = filepath.Join(store.dataDir, "ml_training_data.bin")
	store.Add(TrainingSample{Comm: "echo", Args: []string{"safe"}, Timestamp: time.Unix(1700000000, 0)})
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	info, err := os.Lstat(store.persistPath)
	if err != nil {
		t.Fatalf("stat persisted store: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("persisted mode = %v, want regular 0600", info.Mode())
	}
	entries, err := os.ReadDir(store.dataDir)
	if err != nil {
		t.Fatalf("read store directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(store.persistPath) {
		t.Fatalf("store directory entries = %#v", entries)
	}
}

func TestTrainingDataStoreConcurrentAddSnapshotAndFlush(t *testing.T) {
	store := NewTrainingDataStore(64)
	store.dataDir = t.TempDir()
	store.persistPath = filepath.Join(store.dataDir, "ml_training_data.bin")
	store.Add(TrainingSample{Comm: "seed", Args: []string{"0"}, Timestamp: time.Unix(1700000000, 0)})

	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := 0; index < 80; index++ {
				store.Add(TrainingSample{
					Comm:      "worker",
					Args:      []string{string(rune('a' + worker)), strings.Repeat("x", index%32)},
					Timestamp: time.Unix(1700000001+int64(worker*100+index), 0),
				})
				_ = store.AllSamples()
				_ = store.AllSamplesWithIndex()
				_ = store.BoundedSamplesWithIndex(8, false)
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		for index := 0; index < 8; index++ {
			if err := store.Flush(); err != nil {
				t.Errorf("concurrent Flush() error = %v", err)
				return
			}
		}
	}()
	workers.Wait()
	if err := store.Flush(); err != nil {
		t.Fatalf("final Flush() error = %v", err)
	}

	loaded := NewTrainingDataStore(64)
	loaded.dataDir = store.dataDir
	loaded.persistPath = store.persistPath
	if err := loaded.LoadFromDisk(); err != nil {
		t.Fatalf("load concurrent snapshot: %v", err)
	}
	if total, _ := loaded.Status(); total == 0 || total > 64 {
		t.Fatalf("loaded sample count = %d, want 1..64", total)
	}
}

func TestTrainingStoreLimitWriterRejectsOverflowWithoutPartialWrite(t *testing.T) {
	var destination bytes.Buffer
	writer := &trainingStoreLimitWriter{writer: &destination, remaining: 3}
	written, err := writer.Write([]byte("four"))
	if !errors.Is(err, errTrainingStoreTooLarge) || written != 0 || destination.Len() != 0 {
		t.Fatalf("Write() = (%d, %v), destination bytes = %d", written, err, destination.Len())
	}
}

func TestResolveTrainingStoreTargetRejectsOutsidePath(t *testing.T) {
	directory := t.TempDir()
	outside := filepath.Join(filepath.Dir(directory), "outside.bin")
	if _, _, err := resolveTrainingStoreTarget(directory, outside); !errors.Is(err, errTrainingStorePathOutsideRoot) {
		t.Fatalf("resolveTrainingStoreTarget() error = %v, want path-outside-root error", err)
	}
}

func assertBoundedTrainingString(t *testing.T, name, value string, maxBytes int) {
	t.Helper()
	if len(value) > maxBytes {
		t.Fatalf("%s bytes = %d, want <= %d", name, len(value), maxBytes)
	}
	if !utf8.ValidString(value) {
		t.Fatalf("%s is not valid UTF-8", name)
	}
}

func assertSingleTrainingCommand(t *testing.T, store *TrainingDataStore, command string) {
	t.Helper()
	samples := store.AllSamples()
	if len(samples) != 1 || samples[0].Comm != command {
		t.Fatalf("stored samples = %#v, want one %q command", samples, command)
	}
}

func writeTestTrainingWirePayload(t *testing.T, destination *bytes.Buffer, payload []byte) {
	t.Helper()
	if len(payload) > trainingStoreMaxWireStringBytes {
		t.Fatalf("test payload too large: %d", len(payload))
	}
	if err := binary.Write(destination, binary.LittleEndian, uint16(len(payload))); err != nil {
		t.Fatalf("write payload length: %v", err)
	}
	if _, err := destination.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
}
