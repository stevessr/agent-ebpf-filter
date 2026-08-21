// Package recording — JSONL event recording: lifecycle state machine,
// background flush worker, file layout helpers and replay reading.
//
// Bridge file: type aliases so moved files keep their original identifiers.

package recording

import "agent-ebpf-filter/core"

type CapturedEventRecord = core.CapturedEventRecord
