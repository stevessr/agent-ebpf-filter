package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// ebpfModule mirrors the backend plugin manifest's runtime-facing fields.
// These are independently attachable user eBPF plugins, not the built-in
// collector's event-type filters.
type ebpfModule struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Kind         string `json:"kind"`
	Enabled      bool   `json:"enabled"`
	Loaded       bool   `json:"loaded"`
	LoadError    string `json:"loadError"`
	AttachKind   string `json:"attachKind"`
	AttachTarget string `json:"attachTarget"`
	ProgramName  string `json:"programName"`
}

func (c *apiClient) ebpfModules(ctx context.Context) ([]ebpfModule, error) {
	var response struct {
		Plugins []ebpfModule `json:"plugins"`
	}
	if err := c.getJSON(ctx, "/plugins", &response); err != nil {
		return nil, err
	}
	modules := make([]ebpfModule, 0, len(response.Plugins))
	for _, plugin := range response.Plugins {
		if plugin.Kind == "ebpf" && plugin.ID != "" {
			modules = append(modules, plugin)
		}
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].ID < modules[j].ID })
	return modules, nil
}

// setEBPFModuleLoaded controls only the selected registered eBPF plugin.
// Do not call event-type APIs here: those filter events without detaching
// kernel programs. The backend validates permissions and owns the eBPF links.
func (c *apiClient) setEBPFModuleLoaded(ctx context.Context, id string, load bool) (ebpfModule, error) {
	if strings.TrimSpace(id) == "" {
		return ebpfModule{}, fmt.Errorf("eBPF module ID is required")
	}
	path := "/plugins/bpf/unload"
	if load {
		path = "/plugins/bpf/load"
	}
	var response struct {
		Plugin ebpfModule `json:"plugin"`
	}
	if err := c.requestJSON(ctx, http.MethodPost, path, map[string]string{"id": id}, &response); err != nil {
		return ebpfModule{}, err
	}
	if response.Plugin.ID != id || response.Plugin.Kind != "ebpf" {
		return ebpfModule{}, fmt.Errorf("%s: backend returned an unexpected module", path)
	}
	if response.Plugin.Loaded != load {
		return ebpfModule{}, fmt.Errorf("%s: backend did not confirm the requested eBPF state", path)
	}
	return response.Plugin, nil
}
