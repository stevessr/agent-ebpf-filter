package app

import "testing"

func TestNormalizeBackendWorkerQueueSize(t *testing.T) {
	tests := []struct {
		name         string
		queueSize    int
		defaultSize  int
		expectedSize int
	}{
		{name: "default", queueSize: 0, defaultSize: 2048, expectedSize: 2048},
		{name: "invalid default", queueSize: 0, defaultSize: 0, expectedSize: 1},
		{name: "small explicit queue", queueSize: 8, defaultSize: 2048, expectedSize: 8},
		{name: "bounded explicit queue", queueSize: backendWorkerMaxQueueSize + 1, defaultSize: 2048, expectedSize: backendWorkerMaxQueueSize},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeBackendWorkerQueueSize(test.queueSize, test.defaultSize); got != test.expectedSize {
				t.Fatalf("normalizeBackendWorkerQueueSize(%d, %d) = %d, want %d", test.queueSize, test.defaultSize, got, test.expectedSize)
			}
		})
	}
}
