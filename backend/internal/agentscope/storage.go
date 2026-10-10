package agentscope

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadFile reads a previously stored scope policy and validates all entries
// before returning it. The caller owns fallback/default and error reporting.
func LoadFile(path string) (Policy, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	var cfg Policy
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Policy{}, fmt.Errorf("decode scope policy: %w", err)
	}
	cfg, err = Validate(cfg)
	if err != nil {
		return Policy{}, fmt.Errorf("validate scope policy: %w", err)
	}
	return cfg, nil
}

// SaveFile preserves the previous atomic temp-write/rename storage format.
// The runtime directory remains owned and access-controlled by the caller.
func SaveFile(path string, cfg Policy) error {
	// Never persist an invalid scope policy, even when called without HTTP.
	var err error
	cfg, err = Validate(cfg)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".agent-scopes-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
