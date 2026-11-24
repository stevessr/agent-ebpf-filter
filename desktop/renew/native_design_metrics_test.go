package main

import (
	"testing"

	"github.com/stevessr/agent-ebpf-filter/desktop/renew/design"
)

func TestOverviewMetricSpecsPreserveEvidenceScope(t *testing.T) {
	metrics := overviewMetricSpecs(12, 3, 2)
	if len(metrics) != 3 || metrics[0].Value != "12" ||
		metrics[1].Value != "2" || metrics[2].Value != "3" {
		t.Fatalf("incorrect bounded event summary: %+v", metrics)
	}
	if metrics[1].Tone != design.Danger || metrics[2].Tone != design.Warning {
		t.Fatalf("nonzero alert counts must carry explicit severity: %+v", metrics)
	}
	for _, metric := range metrics {
		if metric.Detail == "" {
			t.Fatalf("metric %q omitted its evidence scope", metric.Label)
		}
	}
	empty := overviewMetricSpecs(0, 0, 0)
	if empty[1].Tone == design.Success || empty[2].Tone == design.Success {
		t.Fatal("zero alerts cannot imply successful protection")
	}
}
