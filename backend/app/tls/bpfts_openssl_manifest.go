package tls

import (
	"fmt"
	"path/filepath"

	"agent-ebpf-filter/app/bpfts"
)

func validateBpfTSOpenSSLManifest(manifest bpfts.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	if filepath.Base(manifest.Source) != "tls-openssl.ts" {
		return fmt.Errorf("bpf-ts OpenSSL ABI requires source tls-openssl.ts, got %q", manifest.Source)
	}

	expectedProbes := map[string]bpfts.ManifestProbe{
		"write": {
			Name: "write", Section: "uprobe", Kind: "uprobe", Target: "SSL_write",
		},
		"readEnter": {
			Name: "readEnter", Section: "uprobe", Kind: "uprobe", Target: "SSL_read",
		},
		"readComplete": {
			Name: "readComplete", Section: "uretprobe", Kind: "uretprobe", Target: "SSL_read",
		},
	}
	if len(manifest.Probes) != len(expectedProbes) {
		return fmt.Errorf("bpf-ts OpenSSL ABI requires exactly %d probes, got %d", len(expectedProbes), len(manifest.Probes))
	}
	for _, probe := range manifest.Probes {
		expected, ok := expectedProbes[probe.Name]
		if !ok || probe != expected {
			return fmt.Errorf("bpf-ts OpenSSL ABI rejects unexpected probe %+v", probe)
		}
	}

	expectedMaps := map[string]bpfts.ManifestMap{
		"pendingReadBuffers": {
			Name: "pendingReadBuffers", Kind: "hash", MaxEntries: 16384,
		},
		"pendingReadConnections": {
			Name: "pendingReadConnections", Kind: "hash", MaxEntries: 16384,
		},
		bpfTSOpenSSLRingName: {
			Name: bpfTSOpenSSLRingName, Kind: "ringbuf", MaxEntries: 1 << 20,
		},
		bpfTSOpenSSLScratchName: {
			Name: bpfTSOpenSSLScratchName, Kind: "percpu_array", MaxEntries: 1,
		},
		bpfTSOpenSSLDropName: {
			Name: bpfTSOpenSSLDropName, Kind: "percpu_array", MaxEntries: 1,
		},
	}
	if len(manifest.Maps) != len(expectedMaps) {
		return fmt.Errorf("bpf-ts OpenSSL ABI requires exactly %d maps, got %d", len(expectedMaps), len(manifest.Maps))
	}
	for _, item := range manifest.Maps {
		expected, ok := expectedMaps[item.Name]
		if !ok || item != expected {
			return fmt.Errorf("bpf-ts OpenSSL ABI rejects unexpected map %+v", item)
		}
	}
	return nil
}
