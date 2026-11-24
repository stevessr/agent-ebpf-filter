package app

import (
	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/app/types"
	"agent-ebpf-filter/pb"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// trackingConfigStore exposes the app's tracking configuration (tag registry,
// disabled comms / event types, wrapper rules) to the handlers package.
type trackingConfigStore struct{}

func (trackingConfigStore) TagID(name string) uint32 { return getTagID(name) }
func (trackingConfigStore) TagName(id uint32) string { return getTagName(id) }

func (trackingConfigStore) TagNames() []string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	names := make([]string, 0, len(tagMap))
	for _, name := range tagMap {
		names = append(names, name)
	}
	return names
}

func (trackingConfigStore) IsCommDisabled(comm string) bool {
	disabledCommsMu.RLock()
	defer disabledCommsMu.RUnlock()
	_, ok := disabledComms[comm]
	return ok
}

func (trackingConfigStore) AddDisabledComm(comm string) {
	disabledCommsMu.Lock()
	disabledComms[comm] = struct{}{}
	disabledCommsMu.Unlock()
}

func (trackingConfigStore) RemoveDisabledComm(comm string) {
	disabledCommsMu.Lock()
	delete(disabledComms, comm)
	disabledCommsMu.Unlock()
}

func (trackingConfigStore) DisabledEventTypes() []uint32 {
	disabledEventTypesMu.RLock()
	defer disabledEventTypesMu.RUnlock()
	disabled := make([]uint32, 0, len(disabledEventTypes))
	for eventType := range disabledEventTypes {
		disabled = append(disabled, eventType)
	}
	return disabled
}

func (trackingConfigStore) ReplaceDisabledEventTypes(eventTypes []uint32) {
	next := make(map[uint32]struct{}, len(eventTypes))
	var words [4]uint64
	for _, eventType := range eventTypes {
		next[eventType] = struct{}{}
		if eventType <= 255 {
			words[eventType>>6] |= uint64(1) << (eventType & 63)
		}
	}
	disabledEventTypesMu.Lock()
	disabledEventTypes = next
	for index := range words {
		disabledEventTypeBits[index].Store(words[index])
	}
	disabledEventTypesMu.Unlock()
}

func (trackingConfigStore) AddDisabledEventType(eventType uint32) {
	disabledEventTypesMu.Lock()
	disabledEventTypes[eventType] = struct{}{}
	if eventType <= 255 {
		index := eventType >> 6
		bit := uint64(1) << (eventType & 63)
		disabledEventTypeBits[index].Store(disabledEventTypeBits[index].Load() | bit)
	}
	disabledEventTypesMu.Unlock()
}

func (trackingConfigStore) RemoveDisabledEventType(eventType uint32) {
	disabledEventTypesMu.Lock()
	delete(disabledEventTypes, eventType)
	if eventType <= 255 {
		index := eventType >> 6
		bit := uint64(1) << (eventType & 63)
		disabledEventTypeBits[index].Store(disabledEventTypeBits[index].Load() &^ bit)
	}
	disabledEventTypesMu.Unlock()
}

func (trackingConfigStore) Rules() []*pb.WrapperRule {
	rulesMu.RLock()
	defer rulesMu.RUnlock()
	result := make([]*pb.WrapperRule, 0, len(wrapperRules))
	for _, r := range wrapperRules {
		result = append(result, &pb.WrapperRule{
			Comm:         r.Comm,
			Action:       r.Action,
			RewrittenCmd: r.RewrittenCmd,
			Regex:        r.Regex,
			Replacement:  r.Replacement,
			Priority:     int32(r.Priority),
		})
	}
	return result
}

func (trackingConfigStore) UpsertRule(comm, action string, rewrittenCmd []string, regex, replacement string, priority int32) {
	rulesMu.Lock()
	wrapperRules[comm] = WrapperRule{
		Comm:         comm,
		Action:       action,
		RewrittenCmd: append([]string(nil), rewrittenCmd...),
		Regex:        regex,
		Replacement:  replacement,
		Priority:     int(priority),
	}
	rulesMu.Unlock()
}

func (trackingConfigStore) DeleteRule(comm string) {
	rulesMu.Lock()
	delete(wrapperRules, comm)
	rulesMu.Unlock()
}

// pluginService exposes the plugin registry and eBPF builder to the handlers
// package.
type pluginService struct{}

func (pluginService) ValidateID(id string) error { return validatePluginID(id) }

func (pluginService) List() []types.PluginManifest { return pluginRegistry.List() }

func (pluginService) Get(id string) (types.PluginManifest, bool) { return pluginRegistry.Get(id) }

func (pluginService) Source(id string) (string, bool) {
	source, err := PluginSource(id)
	return source, err == nil
}

func (pluginService) Upsert(req *handlers.PluginUpsertRequest) (types.PluginManifest, error) {
	kind := PluginKind(strings.TrimSpace(req.Kind))
	if kind == "" {
		kind = PluginKindEBPF
	}
	m := &PluginManifest{
		ID:             strings.TrimSpace(req.ID),
		Name:           sanitizePluginName(req.Name),
		Description:    strings.TrimSpace(req.Description),
		Author:         strings.TrimSpace(req.Author),
		Version:        strings.TrimSpace(req.Version),
		Kind:           kind,
		Enabled:        req.Enabled,
		AttachKind:     PluginAttachKind(strings.TrimSpace(req.AttachKind)),
		AttachTarget:   strings.TrimSpace(req.AttachTarget),
		ProgramName:    strings.TrimSpace(req.ProgramName),
		WebhookURL:     strings.TrimSpace(req.WebhookURL),
		WebhookEvents:  append([]string(nil), req.WebhookEvents...),
		CommandComm:    strings.TrimSpace(req.CommandComm),
		CommandArgs:    append([]string(nil), req.CommandArgs...),
		CommandRule:    strings.TrimSpace(req.CommandRule),
		CommandRewrite: append([]string(nil), req.CommandRewrite...),
	}
	var err error
	if kind == PluginKindEBPF && strings.TrimSpace(req.Source) != "" {
		err = pluginRegistry.UpsertWithSource(m, req.Source)
	} else {
		err = pluginRegistry.Upsert(m)
	}
	if err != nil {
		return types.PluginManifest{}, err
	}
	stored, _ := pluginRegistry.Get(m.ID)
	return stored, nil
}

func (pluginService) Delete(id string) error { return pluginRegistry.Delete(id) }

func (pluginService) SetEnabled(ctx context.Context, id string, enabled bool) (types.PluginManifest, error) {
	manifest, err := pluginRegistry.SetEnabled(id, enabled)
	if err != nil {
		return types.PluginManifest{}, err
	}
	if manifest.Kind == PluginKindEBPF {
		if enabled {
			if err := LoadEBPFPluginContext(ctx, &manifest); err != nil {
				return manifest, err
			}
		} else {
			UnloadEBPFPlugin(id)
		}
	}
	stored, _ := pluginRegistry.Get(id)
	return stored, nil
}

func (pluginService) LoadEBPF(ctx context.Context, id string) (types.PluginManifest, error) {
	manifest, ok := pluginRegistry.Get(id)
	if !ok {
		return types.PluginManifest{}, fmt.Errorf("plugin %q not found", id)
	}
	if manifest.Kind != PluginKindEBPF {
		return types.PluginManifest{}, errors.New("not an eBPF plugin")
	}
	if err := LoadEBPFPluginContext(ctx, &manifest); err != nil {
		return types.PluginManifest{}, err
	}
	updated, _ := pluginRegistry.Get(id)
	return updated, nil
}

func (pluginService) UnloadEBPF(id string) { UnloadEBPFPlugin(id) }

func (pluginService) BPFTemplates() []types.BPFTemplate { return bpfTemplates() }

func (pluginService) CompileUserBPF(ctx context.Context, id, source string) (string, []byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if err := validateUserBPFSource(source); err != nil {
		return "", nil, err
	}
	manifest, exists := pluginRegistry.Get(id)
	if !exists {
		manifest = PluginManifest{
			ID:         id,
			Name:       id,
			Kind:       PluginKindEBPF,
			AttachKind: PluginAttachNone,
		}
	} else if manifest.Kind != PluginKindEBPF {
		return "", nil, errors.New("not an eBPF plugin")
	} else if manifest.Enabled {
		return "", nil, errors.New("disable the eBPF plugin before recompiling it")
	}
	if err := pluginRegistry.UpsertWithSourceContext(ctx, &manifest, source); err != nil {
		return "", nil, fmt.Errorf("prepare plugin source: %w", err)
	}
	objectPath, diagnostics, err := CompileUserBPFContext(ctx, id, source)
	if err != nil {
		return objectPath, diagnostics, err
	}
	object, err := readPluginFile(id, "program.o", maxUserBPFObjectBytes)
	if err != nil {
		return objectPath, diagnostics, fmt.Errorf("read compiled plugin object: %w", err)
	}
	if err := pluginRegistry.RecordCompile(id, sha256Hex([]byte(source)), sha256Hex(object)); err != nil {
		return objectPath, diagnostics, fmt.Errorf("record compiled plugin: %w", err)
	}
	return objectPath, diagnostics, nil
}

// mlService exposes the ML engine to the handlers package.
type mlService struct{}

func (mlService) Status() *pb.MLStatus { return mlStatus() }
func (mlService) StatusJSON() []byte   { return buildMLStatusJSON() }
func (mlService) Enabled() bool        { return ml.SnapshotMLRuntime().Enabled }
func (mlService) IsTraining() bool     { return ml.GlobalTrainer.IsRunning() }
func (mlService) CancelTraining()      { cancelMLAutoTuneTasks() }
func (mlService) Logs() gin.H          { return mlGetLogsResponse() }
func (mlService) History() gin.H       { return mlGetHistoryResponse() }
func (mlService) Train(numTrees, maxDepth, minSamplesLeaf int) gin.H {
	return mlTrain(numTrees, maxDepth, minSamplesLeaf)
}
func (mlService) Feedback(comm, userAction string) gin.H { return mlFeedbackResult(comm, userAction) }
func (mlService) Samples() gin.H                         { return mlSamplesResponse() }
func (mlService) LabelSample(index int, label string) gin.H {
	return mlSampleLabelResult(index, label)
}
func (mlService) RemoveSample(index int) gin.H { return mlRemoveSampleResult(index) }
func (mlService) SetSampleAnomaly(index int, score float64) gin.H {
	return mlSampleAnomalyResult(index, score)
}

func (mlService) AddSample(commandLine, comm string, args []string, label string) gin.H {
	return mlAddSample(commandLine, comm, args, label)
}
