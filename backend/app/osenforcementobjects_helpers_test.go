package app

import (
	"os"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
)

func readSourceFiles(t *testing.T, paths ...string) string {
	t.Helper()
	var builder strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func assertProgramSpec(t *testing.T, spec *ebpf.CollectionSpec, name string, typ ebpf.ProgramType, attach ebpf.AttachType, section string) {
	t.Helper()
	prog, ok := spec.Programs[name]
	if !ok {
		t.Fatalf("missing program %s", name)
	}
	if prog.Type != typ || prog.AttachType != attach || prog.SectionName != section {
		t.Fatalf("program %s = type %s attach %s section %q, want type %s attach %s section %q",
			name, prog.Type, prog.AttachType, prog.SectionName, typ, attach, section)
	}
	if len(prog.Instructions) == 0 {
		t.Fatalf("program %s has no instructions", name)
	}
}

func assertMapSpec(t *testing.T, spec *ebpf.CollectionSpec, name string, typ ebpf.MapType, maxEntries, keySize, valueSize uint32) {
	t.Helper()
	m, ok := spec.Maps[name]
	if !ok {
		t.Fatalf("missing map %s", name)
	}
	if m.Type != typ || m.MaxEntries != maxEntries || m.KeySize != keySize || m.ValueSize != valueSize {
		t.Fatalf("map %s = type %s max_entries %d key_size %d value_size %d, want type %s max_entries %d key_size %d value_size %d",
			name, m.Type, m.MaxEntries, m.KeySize, m.ValueSize, typ, maxEntries, keySize, valueSize)
	}
}

func assertProgramReferencesMap(t *testing.T, spec *ebpf.CollectionSpec, progName, mapName string) {
	t.Helper()
	prog, ok := spec.Programs[progName]
	if !ok {
		t.Fatalf("missing program %s", progName)
	}
	for _, ins := range prog.Instructions {
		if ins.Reference() == mapName {
			return
		}
	}
	t.Fatalf("program %s does not reference map %s", progName, mapName)
}

func stringFromNULBytes(b []byte) string {
	if idx := strings.IndexByte(string(b), 0); idx >= 0 {
		return string(b[:idx])
	}
	return string(b)
}
