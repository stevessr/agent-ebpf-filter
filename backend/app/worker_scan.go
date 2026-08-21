package app

import "agent-ebpf-filter/app/tasks"

// backendWorkerScanBatchSize bounds temporary allocations and lock hold times
// while workers process large manual replay requests.
const backendWorkerScanBatchSize = tasks.ScanBatchSize

const backendWorkerMaxQueueSize = tasks.MaxQueueSize

var normalizeBackendWorkerQueueSize = tasks.NormalizeQueueSize
