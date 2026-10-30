package kernelguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHoldoutSourceDigestBindsExactCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.csv")
	initial := []byte("agent_run_id,destination_ip,destination_port,label\nrun-sample,8.8.8.8,4444,attack\n")
	if err := os.WriteFile(path, initial, 0600); err != nil {
		t.Fatal(err)
	}
	first, digest1, err := evaluateCSVHoldoutWithDigest(path, sampleModel())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(initial, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	second, digest2, err := evaluateCSVHoldoutWithDigest(path, sampleModel())
	if err != nil {
		t.Fatal(err)
	}
	if digest1 == digest2 {
		t.Fatal("different CSV byte streams must not share an approval digest")
	}
	if first != second {
		t.Fatalf("adding a blank row unexpectedly changed the replay classification: %+v vs %+v", first, second)
	}
}
