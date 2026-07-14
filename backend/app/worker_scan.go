package app

// backendWorkerScanBatchSize bounds temporary allocations and lock hold times
// while workers process large manual replay requests.
const backendWorkerScanBatchSize = 256

const backendWorkerMaxQueueSize = 65536

func normalizeBackendWorkerQueueSize(queueSize, defaultSize int) int {
	if defaultSize <= 0 {
		defaultSize = 1
	}
	if queueSize <= 0 {
		queueSize = defaultSize
	}
	if queueSize > backendWorkerMaxQueueSize {
		queueSize = backendWorkerMaxQueueSize
	}
	return queueSize
}
