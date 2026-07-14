package executiongraph

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"agent-ebpf-filter/pb"
)

func TestBuildBoundsOversizedFieldsWithStableDistinctIDs(t *testing.T) {
	commonPath := "/tmp/" + strings.Repeat("路径", graphNodeIDMaxBytes)
	largeValue := string([]byte{0xff}) + strings.Repeat("数据", graphMetadataValueMaxBytes)
	records := []Record{
		{
			ReceivedAt: time.Unix(1, 0).UTC(),
			Event: &pb.Event{
				Pid:            1,
				Comm:           largeValue,
				Type:           "openat",
				Path:           commonPath + "-a",
				ExtraInfo:      largeValue,
				AgentRunId:     largeValue + "-run",
				ToolCallId:     largeValue + "-tool",
				ToolName:       largeValue,
				TraceId:        largeValue,
				ConversationId: largeValue,
				TurnId:         largeValue,
			},
		},
		{
			ReceivedAt: time.Unix(2, 0).UTC(),
			Event:      &pb.Event{Pid: 2, Type: "openat", Path: commonPath + "-b"},
		},
	}

	graph, err := BuildContext(context.Background(), records, Filters{})
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if !graph.Truncated || graph.TruncatedFieldCount == 0 {
		t.Fatalf("oversized graph truncation = %#v, want truncated fields", graph)
	}
	assertExecutionGraphBounds(t, graph)

	fileIDs := make(map[string]struct{})
	for _, node := range graph.Nodes {
		if node.Kind == "file" {
			fileIDs[node.ID] = struct{}{}
		}
	}
	if len(fileIDs) != 2 {
		t.Fatalf("oversized paths collapsed into %d file IDs, want 2", len(fileIDs))
	}

	again, err := BuildContext(context.Background(), records, Filters{})
	if err != nil {
		t.Fatalf("second BuildContext() error = %v", err)
	}
	if !reflect.DeepEqual(graph, again) {
		t.Fatal("bounded graph output is not deterministic")
	}
}

func TestBuildCapsInputAndOutputCardinality(t *testing.T) {
	records := make([]Record, graphMaxInputRecords+100)
	for index := range records {
		pid := uint32(index + 1)
		records[index] = Record{
			ReceivedAt: time.Unix(0, int64(index+1)).UTC(),
			Event: &pb.Event{
				Pid:  pid,
				Type: "openat",
				Path: fmt.Sprintf("/tmp/cardinality-%d", index),
			},
		}
	}

	graph, err := BuildContext(context.Background(), records, Filters{})
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if graph.EventCount != graphMaxInputRecords || graph.OmittedEventCount != 100 {
		t.Fatalf("event counts = matched %d omitted %d, want %d/100", graph.EventCount, graph.OmittedEventCount, graphMaxInputRecords)
	}
	if !graph.Truncated || graph.OmittedNodeCount == 0 {
		t.Fatalf("cardinality graph did not report omitted nodes: %#v", graph)
	}
	assertExecutionGraphBounds(t, graph)
}

func TestBuildCapsEncodedOutputBudget(t *testing.T) {
	largeValue := strings.Repeat("x", graphMetadataValueMaxBytes+1024)
	records := make([]Record, 1000)
	for index := range records {
		records[index] = Record{
			ReceivedAt: time.Unix(0, int64(index+1)).UTC(),
			Event: &pb.Event{
				Pid:       1,
				Comm:      "agent",
				Type:      "semantic_alert",
				Path:      largeValue,
				ExtraInfo: largeValue,
				Decision:  "ALERT",
			},
		}
	}

	graph, err := BuildContext(context.Background(), records, Filters{})
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if !graph.Truncated || graph.OmittedNodeCount == 0 || graph.OmittedEdgeCount == 0 {
		t.Fatalf("encoded budget was not observable: truncated=%t omittedNodes=%d omittedEdges=%d", graph.Truncated, graph.OmittedNodeCount, graph.OmittedEdgeCount)
	}
	assertExecutionGraphBounds(t, graph)
	payload, err := json.Marshal(graph)
	if err != nil {
		t.Fatalf("json.Marshal(graph) error = %v", err)
	}
	if len(payload) > int(graphMaxEncodedBytes) {
		t.Fatalf("encoded graph size = %d, want at most %d", len(payload), graphMaxEncodedBytes)
	}
}

func TestExtractGraphPIDRejectsOverflowAndHandlesUnicodeWhitespace(t *testing.T) {
	if pid, ok := extractGraphPID("noise=1\u2003child_pid=42", "child_pid"); !ok || pid != 42 {
		t.Fatalf("unicode-space PID = %d, %t, want 42, true", pid, ok)
	}
	if pid, ok := extractGraphPID("child_pid=4294967296", "child_pid"); ok || pid != 0 {
		t.Fatalf("overflow PID = %d, %t, want 0, false", pid, ok)
	}
}

func TestExtractGraphStringDenseInputDoesNotAllocateFieldSlice(t *testing.T) {
	extraInfo := strings.Repeat("noise=1 ", 10000) + "newpath=/tmp/result"
	allocations := testing.AllocsPerRun(20, func() {
		value, ok := extractGraphString(extraInfo, "newpath")
		if !ok || value != "/tmp/result" {
			panic("dense field extraction failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("dense field extraction allocations = %.2f, want 0", allocations)
	}
}

func assertExecutionGraphBounds(t *testing.T, graph Response) {
	t.Helper()
	if len(graph.Nodes) > graphMaxNodes {
		t.Fatalf("node count = %d, limit %d", len(graph.Nodes), graphMaxNodes)
	}
	if len(graph.Edges) > graphMaxEdges {
		t.Fatalf("edge count = %d, limit %d", len(graph.Edges), graphMaxEdges)
	}
	nodeIDs := make(map[string]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if len(node.ID) > graphNodeIDMaxBytes || !utf8.ValidString(node.ID) {
			t.Fatalf("invalid node ID length=%d valid=%t", len(node.ID), utf8.ValidString(node.ID))
		}
		if len(node.Kind) > graphKindMaxBytes || len(node.Label) > graphLabelMaxBytes || len(node.Subtitle) > graphSubtitleMaxBytes {
			t.Fatalf("node text exceeds bounds: kind=%d label=%d subtitle=%d", len(node.Kind), len(node.Label), len(node.Subtitle))
		}
		for key, value := range node.Metadata {
			if len(key) > graphMetadataKeyMaxBytes || len(value) > graphMetadataValueMaxBytes || !utf8.ValidString(key) || !utf8.ValidString(value) {
				t.Fatalf("metadata exceeds bounds: key=%d value=%d valid=%t/%t", len(key), len(value), utf8.ValidString(key), utf8.ValidString(value))
			}
			if strings.TrimSpace(value) == "" {
				t.Fatalf("empty metadata value was retained for key %q", key)
			}
		}
		nodeIDs[node.ID] = struct{}{}
	}
	for _, edge := range graph.Edges {
		if len(edge.ID) > graphEdgeIDMaxBytes || len(edge.Source) > graphNodeIDMaxBytes || len(edge.Target) > graphNodeIDMaxBytes || len(edge.Kind) > graphKindMaxBytes || len(edge.Label) > graphEdgeLabelMaxBytes {
			t.Fatalf("edge exceeds bounds: %#v", edge)
		}
		if _, ok := nodeIDs[edge.Source]; !ok {
			t.Fatalf("edge %q has omitted source %q", edge.ID, edge.Source)
		}
		if _, ok := nodeIDs[edge.Target]; !ok {
			t.Fatalf("edge %q has omitted target %q", edge.ID, edge.Target)
		}
	}
}
