package main

import (
	"reflect"
	"testing"
)

func TestMonitoringProfileDisabledPreservesExternalTypes(t *testing.T) {
	current := map[uint32]bool{41: true, 43: true, 1: true}
	daily, ok := monitoringProfileByKey("daily")
	if !ok {
		t.Fatal("daily profile missing")
	}
	got := monitoringProfileDisabled(current, daily)
	if !got[41] || !got[43] {
		t.Fatalf("external disabled types were lost: %#v", got)
	}
	for _, eventType := range []uint32{0, 18, 3, 10, 2, 31, 34} {
		if got[eventType] {
			t.Fatalf("daily event type %d unexpectedly disabled", eventType)
		}
	}
	for _, eventType := range []uint32{1, 5, 9, 11, 7, 8, 20, 21, 22, 33, 25} {
		if !got[eventType] {
			t.Fatalf("deep-only event type %d should be disabled", eventType)
		}
	}
}

func TestDisabledEventTypeSetFromRuntimeJSONShape(t *testing.T) {
	got := disabledEventTypeSet(map[string]any{
		"disabledEventTypes": []any{float64(1), float64(25), float64(41)},
	})
	want := map[uint32]bool{1: true, 25: true, 41: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("disabled set = %#v, want %#v", got, want)
	}
}

func TestRuntimeNestedPatchPreservesSettings(t *testing.T) {
	runtime := map[string]any{
		"loopDetection": map[string]any{
			"enabled":            false,
			"windowSeconds":      float64(30),
			"repeatThreshold":    float64(4),
			"emitSemanticAlerts": true,
		},
	}
	got := runtimeNestedPatch(runtime, "loopDetection", true)
	if got["enabled"] != true || got["windowSeconds"] != float64(30) || got["repeatThreshold"] != float64(4) {
		t.Fatalf("nested runtime patch lost settings: %#v", got)
	}
	if runtimeNestedEnabled(runtime, "loopDetection") {
		t.Fatal("source runtime map was mutated")
	}
}

func TestParseRewriteArgs(t *testing.T) {
	got, err := parseRewriteArgs(`["echo","hello world"]`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"echo", "hello world"}) {
		t.Fatalf("rewrite args = %#v", got)
	}
	for _, raw := range []string{`[]`, `[""]`, `"echo hi"`} {
		if _, err := parseRewriteArgs(raw); err == nil {
			t.Fatalf("parseRewriteArgs(%q) expected error", raw)
		}
	}
}
