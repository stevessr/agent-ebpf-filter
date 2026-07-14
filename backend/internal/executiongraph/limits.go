package executiongraph

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash/maphash"
	"io"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	graphMaxInputRecords       = 10000
	graphMaxNodes              = 12000
	graphMaxEdges              = 24000
	graphMaxEncodedBytes int64 = 32 * 1024 * 1024

	graphNodeIDMaxBytes        = 512
	graphEdgeIDMaxBytes        = 1024
	graphKindMaxBytes          = 64
	graphLabelMaxBytes         = 512
	graphSubtitleMaxBytes      = 1024
	graphMetadataKeyMaxBytes   = 128
	graphMetadataValueMaxBytes = 4096
	graphEdgeLabelMaxBytes     = 128
	graphSanitizedIDMaxBytes   = 128
)

type graphBuilder struct {
	nodes           map[string]Node
	edges           map[string]Edge
	nodeBytes       map[string]int64
	edgeBytes       map[string]int64
	encodedBytes    int64
	omittedEvents   int
	omittedNodeIDs  map[uint64]struct{}
	omittedEdgeIDs  map[uint64]struct{}
	omittedHashSeed maphash.Seed
	truncatedFields int
}

func newGraphBuilder() *graphBuilder {
	return &graphBuilder{
		nodes:           make(map[string]Node),
		edges:           make(map[string]Edge),
		nodeBytes:       make(map[string]int64),
		edgeBytes:       make(map[string]int64),
		omittedNodeIDs:  make(map[uint64]struct{}),
		omittedEdgeIDs:  make(map[uint64]struct{}),
		omittedHashSeed: maphash.MakeSeed(),
	}
}

func (b *graphBuilder) addNode(node Node) Node {
	node, truncated := normalizeGraphNode(node)
	b.truncatedFields += truncated
	if node.ID == "" {
		return node
	}
	if existing, ok := b.nodes[node.ID]; ok {
		merged := mergeGraphNode(existing, node)
		oldBytes := b.nodeBytes[node.ID]
		newBytes := estimateGraphNodeEncodedBytes(merged)
		if !b.canGrow(newBytes - oldBytes) {
			b.truncatedFields++
			return existing
		}
		b.nodes[node.ID] = merged
		b.nodeBytes[node.ID] = newBytes
		b.encodedBytes += newBytes - oldBytes
		return merged
	}

	nodeBytes := estimateGraphNodeEncodedBytes(node)
	if len(b.nodes) >= graphMaxNodes || !b.canGrow(nodeBytes) {
		b.markOmittedNode(node.ID)
		return node
	}
	b.nodes[node.ID] = node
	b.nodeBytes[node.ID] = nodeBytes
	b.encodedBytes += nodeBytes
	return node
}

func (b *graphBuilder) addEdge(edge Edge) Edge {
	edge, truncated := normalizeGraphEdge(edge)
	b.truncatedFields += truncated
	if edge.ID == "" || edge.Source == "" || edge.Target == "" {
		return edge
	}
	if _, ok := b.nodes[edge.Source]; !ok {
		b.markOmittedEdge(edge.ID)
		return edge
	}
	if _, ok := b.nodes[edge.Target]; !ok {
		b.markOmittedEdge(edge.ID)
		return edge
	}
	if existing, ok := b.edges[edge.ID]; ok {
		oldBytes := b.edgeBytes[edge.ID]
		newBytes := estimateGraphEdgeEncodedBytes(edge)
		if !b.canGrow(newBytes - oldBytes) {
			b.truncatedFields++
			return existing
		}
		b.edges[edge.ID] = edge
		b.edgeBytes[edge.ID] = newBytes
		b.encodedBytes += newBytes - oldBytes
		return edge
	}

	edgeBytes := estimateGraphEdgeEncodedBytes(edge)
	if len(b.edges) >= graphMaxEdges || !b.canGrow(edgeBytes) {
		b.markOmittedEdge(edge.ID)
		return edge
	}
	b.edges[edge.ID] = edge
	b.edgeBytes[edge.ID] = edgeBytes
	b.encodedBytes += edgeBytes
	return edge
}

func (b *graphBuilder) canGrow(delta int64) bool {
	return delta <= 0 || (delta <= graphMaxEncodedBytes && b.encodedBytes <= graphMaxEncodedBytes-delta)
}

func (b *graphBuilder) markOmittedNode(id string) {
	if id != "" {
		b.omittedNodeIDs[maphash.String(b.omittedHashSeed, id)] = struct{}{}
	}
}

func (b *graphBuilder) markOmittedEdge(id string) {
	if id != "" {
		b.omittedEdgeIDs[maphash.String(b.omittedHashSeed, id)] = struct{}{}
	}
}

func (b *graphBuilder) response(ctx context.Context, matchedEvents int) (Response, error) {
	nodeList := make([]Node, 0, len(b.nodes))
	nodeCounts := make(map[string]int)
	nodeIndex := 0
	for _, node := range b.nodes {
		if nodeIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return Response{}, err
			}
		}
		nodeList = append(nodeList, node)
		nodeCounts[node.Kind]++
		nodeIndex++
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].Kind == nodeList[j].Kind {
			if nodeList[i].Label == nodeList[j].Label {
				return nodeList[i].ID < nodeList[j].ID
			}
			return nodeList[i].Label < nodeList[j].Label
		}
		return nodeList[i].Kind < nodeList[j].Kind
	})

	edgeList := make([]Edge, 0, len(b.edges))
	edgeCounts := make(map[string]int)
	edgeIndex := 0
	for _, edge := range b.edges {
		if edgeIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return Response{}, err
			}
		}
		edgeList = append(edgeList, edge)
		edgeCounts[edge.Kind]++
		edgeIndex++
	}
	sort.Slice(edgeList, func(i, j int) bool { return edgeList[i].ID < edgeList[j].ID })
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	return Response{
		EventCount:          matchedEvents,
		NodeCounts:          nodeCounts,
		EdgeCounts:          edgeCounts,
		Nodes:               nodeList,
		Edges:               edgeList,
		Truncated:           b.omittedEvents > 0 || len(b.omittedNodeIDs) > 0 || len(b.omittedEdgeIDs) > 0 || b.truncatedFields > 0,
		OmittedEventCount:   b.omittedEvents,
		OmittedNodeCount:    len(b.omittedNodeIDs),
		OmittedEdgeCount:    len(b.omittedEdgeIDs),
		TruncatedFieldCount: b.truncatedFields,
	}, nil
}

func mergeGraphNode(existing, node Node) Node {
	if node.RiskScore > existing.RiskScore {
		existing.RiskScore = node.RiskScore
	}
	if existing.Subtitle == "" && node.Subtitle != "" {
		existing.Subtitle = node.Subtitle
	}
	if (existing.Label == "" || isGenericProcessLabel(existing)) && node.Label != "" && !isGenericProcessLabel(node) {
		existing.Label = node.Label
	}
	if existing.PID == 0 && node.PID != 0 {
		existing.PID = node.PID
	}
	if len(node.Metadata) > 0 {
		metadata := existing.Metadata
		copied := false
		for key, value := range node.Metadata {
			if _, exists := metadata[key]; exists {
				continue
			}
			if !copied {
				cloned := make(map[string]string, len(metadata)+len(node.Metadata))
				for existingKey, existingValue := range metadata {
					cloned[existingKey] = existingValue
				}
				metadata = cloned
				copied = true
			}
			metadata[key] = value
		}
		existing.Metadata = metadata
	}
	return existing
}

func normalizeGraphNode(node Node) (Node, int) {
	truncated := 0
	node.ID, truncated = normalizeGraphValue(node.ID, graphNodeIDMaxBytes, true, truncated)
	node.Kind, truncated = normalizeGraphValue(node.Kind, graphKindMaxBytes, false, truncated)
	node.Label, truncated = normalizeGraphValue(node.Label, graphLabelMaxBytes, false, truncated)
	node.Subtitle, truncated = normalizeGraphValue(node.Subtitle, graphSubtitleMaxBytes, false, truncated)
	if math.IsNaN(node.RiskScore) || math.IsInf(node.RiskScore, 0) {
		node.RiskScore = 0
		truncated++
	}
	if len(node.Metadata) == 0 {
		node.Metadata = nil
		return node, truncated
	}
	metadata := make(map[string]string, len(node.Metadata))
	for key, value := range node.Metadata {
		if strings.TrimSpace(value) == "" {
			continue
		}
		key, truncated = normalizeGraphValue(key, graphMetadataKeyMaxBytes, true, truncated)
		value, truncated = normalizeGraphValue(value, graphMetadataValueMaxBytes, false, truncated)
		if key != "" {
			metadata[key] = value
		}
	}
	if len(metadata) > 0 {
		node.Metadata = metadata
	} else {
		node.Metadata = nil
	}
	return node, truncated
}

func normalizeGraphEdge(edge Edge) (Edge, int) {
	truncated := 0
	edge.ID, truncated = normalizeGraphValue(edge.ID, graphEdgeIDMaxBytes, true, truncated)
	edge.Source, truncated = normalizeGraphValue(edge.Source, graphNodeIDMaxBytes, true, truncated)
	edge.Target, truncated = normalizeGraphValue(edge.Target, graphNodeIDMaxBytes, true, truncated)
	edge.Kind, truncated = normalizeGraphValue(edge.Kind, graphKindMaxBytes, false, truncated)
	edge.Label, truncated = normalizeGraphValue(edge.Label, graphEdgeLabelMaxBytes, false, truncated)
	return edge, truncated
}

func normalizeGraphValue(value string, maxBytes int, stableID bool, truncated int) (string, int) {
	var changed bool
	if stableID {
		value, changed = boundGraphStableID(value, maxBytes)
	} else {
		value, changed = truncateGraphText(value, maxBytes)
	}
	if changed {
		truncated++
	}
	return value, truncated
}

func truncateGraphText(value string, maxBytes int) (string, bool) {
	if len(value) <= maxBytes {
		normalized := strings.ToValidUTF8(value, "\uFFFD")
		if len(normalized) <= maxBytes {
			return normalized, normalized != value
		}
	}
	const suffix = "…"
	headBytes := maxBytes - len(suffix)
	head := strings.ToValidUTF8(graphUTF8Prefix(value, headBytes), "\uFFFD")
	head = graphUTF8Prefix(head, headBytes)
	return head + suffix, true
}

func boundGraphStableID(value string, maxBytes int) (string, bool) {
	if value == "" {
		return "", false
	}
	if len(value) <= maxBytes {
		normalized := strings.ToValidUTF8(value, "\uFFFD")
		if len(normalized) <= maxBytes {
			return normalized, normalized != value
		}
	}
	digest := hashGraphStrings(value)
	suffix := "~" + digest
	headBytes := maxBytes - len(suffix)
	head := strings.ToValidUTF8(graphUTF8Prefix(value, headBytes), "\uFFFD")
	head = graphUTF8Prefix(head, headBytes)
	return head + suffix, true
}

func graphEntityNodeID(prefix, raw string) string {
	separatorBytes := 1
	if len(prefix)+separatorBytes+len(raw) <= graphNodeIDMaxBytes && utf8.ValidString(raw) {
		return prefix + ":" + raw
	}
	digest := hashGraphStrings(prefix, raw)
	suffix := "~" + digest
	headBytes := graphNodeIDMaxBytes - len(prefix) - separatorBytes - len(suffix)
	if headBytes < 0 {
		bounded, _ := boundGraphStableID(prefix+":"+digest, graphNodeIDMaxBytes)
		return bounded
	}
	head := strings.ToValidUTF8(graphUTF8Prefix(raw, headBytes), "\uFFFD")
	head = graphUTF8Prefix(head, headBytes)
	return prefix + ":" + head + suffix
}

func sanitizeGraphID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > graphSanitizedIDMaxBytes {
		digest := hashGraphStrings(value)
		head := graphUTF8Prefix(value, graphSanitizedIDMaxBytes-len(digest)-1)
		return sanitizeGraphID(head) + "~" + digest
	}
	replacer := strings.NewReplacer(
		"/", "_",
		" ", "_",
		":", "_",
		"|", "_",
		"\\", "_",
		"=", "_",
		"?", "_",
		"&", "_",
	)
	normalized := replacer.Replace(strings.ToLower(strings.ToValidUTF8(value, "\uFFFD")))
	bounded, _ := boundGraphStableID(normalized, graphSanitizedIDMaxBytes)
	return bounded
}

func sanitizeGraphIDParts(parts ...string) string {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	if len(parts) > 1 {
		total += len(parts) - 1
	}
	if total <= graphSanitizedIDMaxBytes {
		return sanitizeGraphID(strings.Join(parts, ":"))
	}
	prefix := "value"
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			prefix = sanitizeGraphID(graphUTF8Prefix(part, 32))
			break
		}
	}
	return prefix + "~" + hashGraphStrings(parts...)
}

func joinGraphText(maxBytes int, separator string, parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	total := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		nonEmpty = append(nonEmpty, part)
		total += len(part)
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	total += (len(nonEmpty) - 1) * len(separator)
	if total <= maxBytes {
		return strings.Join(nonEmpty, separator)
	}
	var builder strings.Builder
	builder.Grow(maxBytes)
	remaining := maxBytes - len("…")
	for index, part := range nonEmpty {
		if index > 0 {
			if len(separator) > remaining {
				break
			}
			builder.WriteString(separator)
			remaining -= len(separator)
		}
		prefix := graphUTF8Prefix(part, remaining)
		builder.WriteString(prefix)
		remaining -= len(prefix)
		if len(prefix) < len(part) || remaining == 0 {
			break
		}
	}
	builder.WriteString("…")
	return builder.String()
}

func graphUTF8Prefix(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func hashGraphStrings(parts ...string) string {
	hash := sha256.New()
	var size [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		_, _ = hash.Write(size[:])
		_, _ = io.WriteString(hash, part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func estimateGraphNodeEncodedBytes(node Node) int64 {
	bytes := int64(256)
	bytes += graphEncodedStringUpperBound(node.ID)
	bytes += graphEncodedStringUpperBound(node.Kind)
	bytes += graphEncodedStringUpperBound(node.Label)
	bytes += graphEncodedStringUpperBound(node.Subtitle)
	for key, value := range node.Metadata {
		bytes += 8 + graphEncodedStringUpperBound(key) + graphEncodedStringUpperBound(value)
	}
	return bytes
}

func estimateGraphEdgeEncodedBytes(edge Edge) int64 {
	return 192 + graphEncodedStringUpperBound(edge.ID) + graphEncodedStringUpperBound(edge.Source) + graphEncodedStringUpperBound(edge.Target) + graphEncodedStringUpperBound(edge.Kind) + graphEncodedStringUpperBound(edge.Label)
}

func graphEncodedStringUpperBound(value string) int64 {
	// encoding/json can expand any input byte to at most a six-byte escape.
	return 2 + 6*int64(len(value))
}
