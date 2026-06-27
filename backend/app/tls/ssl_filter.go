package tls

import (
	"strconv"
	"strings"
)

// SSLFilterExpr represents a parsed SSL/TLS filter expression.
// Pattern syntax follows AgentSight ssl_filter.rs:
//
//	condition := field operator value
//	expression := condition | expression & expression | expression | expression
//
// Supported fields: is_handshake, truncated, len, pid, tid, uid, timestamp_ns,
// latency_ms, data_type, direction, lib, function, comm, method, url, host,
// status_code, content_type, body
//
// Supported operators: = (exact), != (not_equal), > (gt), < (lt),
// >= (gte), <= (lte), ~ (contains)
type SSLFilterExpr struct {
	Root filterNode
}

type filterNodeKind int

const (
	filterNodeAnd filterNodeKind = iota
	filterNodeOr
	filterNodeCondition
	filterNodeEmpty
)

type filterNode struct {
	kind      filterNodeKind
	and       *filterPair
	or        *filterPair
	condition *filterCondition
}

type filterPair struct {
	left  *filterNode
	right *filterNode
}

type filterCondition struct {
	field    string
	operator string
	value    string
}

// ParseSSLFilterExpr parses an SSL filter expression string.
func ParseSSLFilterExpr(expression string) *SSLFilterExpr {
	expr := &SSLFilterExpr{}
	expr.Root = parseFilterExpression(strings.TrimSpace(expression))
	return expr
}

// Evaluate tests whether the given event data matches this filter.
func (f *SSLFilterExpr) Evaluate(data map[string]any) bool {
	if f == nil {
		return false
	}
	return evaluateFilterNode(&f.Root, data)
}

// ── Expression parser ───────────────────────────────────────────────

func parseFilterExpression(expr string) filterNode {
	if expr == "" {
		return filterNode{kind: filterNodeEmpty}
	}
	if pos := findFilterOperator(expr, '|'); pos >= 0 {
		return filterNode{
			kind: filterNodeOr,
			or: &filterPair{
				left:  ptrFilterNode(parseFilterExpression(expr[:pos])),
				right: ptrFilterNode(parseFilterExpression(expr[pos+1:])),
			},
		}
	}
	if pos := findFilterOperator(expr, '&'); pos >= 0 {
		return filterNode{
			kind: filterNodeAnd,
			and: &filterPair{
				left:  ptrFilterNode(parseFilterExpression(expr[:pos])),
				right: ptrFilterNode(parseFilterExpression(expr[pos+1:])),
			},
		}
	}
	return parseFilterCondition(expr)
}

func findFilterOperator(expr string, op byte) int {
	depth := 0
	for i := range len(expr) {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if expr[i] == op && depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseFilterCondition(expr string) filterNode {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return parseFilterExpression(expr[1 : len(expr)-1])
	}

	ops := []string{">=", "<=", "!=", "=", ">", "<", "~"}
	for _, op := range ops {
		if pos := strings.Index(expr, op); pos >= 0 {
			field := strings.TrimSpace(expr[:pos])
			rawValue := strings.TrimSpace(expr[pos+len(op):])
			return filterNode{
				kind: filterNodeCondition,
				condition: &filterCondition{
					field:    field,
					operator: operatorFromSymbol(op),
					value:    processEscapeSequences(rawValue),
				},
			}
		}
	}
	return filterNode{kind: filterNodeEmpty}
}

func operatorFromSymbol(sym string) string {
	switch sym {
	case "=":
		return "exact"
	case "!=":
		return "not_equal"
	case ">":
		return "gt"
	case "<":
		return "lt"
	case ">=":
		return "gte"
	case "<=":
		return "lte"
	case "~":
		return "contains"
	default:
		return "exact"
	}
}

func processEscapeSequences(value string) string {
	var b strings.Builder
	chars := []rune(value)
	for i := 0; i < len(chars); i++ {
		if chars[i] == '\\' && i+1 < len(chars) {
			switch chars[i+1] {
			case 'r':
				b.WriteRune('\r')
			case 'n':
				b.WriteRune('\n')
			case 't':
				b.WriteRune('\t')
			case '\\':
				b.WriteRune('\\')
			case '"':
				b.WriteRune('"')
			default:
				b.WriteRune(chars[i])
				b.WriteRune(chars[i+1])
			}
			i++
		} else {
			b.WriteRune(chars[i])
		}
	}
	return b.String()
}

func ptrFilterNode(n filterNode) *filterNode { return &n }

// ── Evaluator ───────────────────────────────────────────────────────

func evaluateFilterNode(node *filterNode, data map[string]any) bool {
	if node == nil {
		return false
	}
	switch node.kind {
	case filterNodeAnd:
		return evaluateFilterNode(node.and.left, data) && evaluateFilterNode(node.and.right, data)
	case filterNodeOr:
		return evaluateFilterNode(node.or.left, data) || evaluateFilterNode(node.or.right, data)
	case filterNodeCondition:
		return evaluateFilterCondition(node.condition, data)
	default:
		return false
	}
}

func evaluateFilterCondition(cond *filterCondition, data map[string]any) bool {
	if cond.field == "data_type" {
		if raw, ok := data["data"].(string); ok {
			return cmpString(DetectSSLDataType(raw), cond.operator, cond.value)
		}
		if raw, ok := data["body"].(string); ok {
			return cmpString(DetectSSLDataType(raw), cond.operator, cond.value)
		}
		return false
	}

	switch cond.field {
	case "is_handshake", "truncated":
		expected := cond.value == "true"
		if v, ok := data[cond.field]; ok {
			if b, ok := v.(bool); ok {
				return b == expected
			}
		}
		return false
	case "len", "pid", "tid", "uid", "timestamp_ns":
		return evalFilterUint64(data, cond.field, cond.operator, cond.value)
	case "latency_ms":
		return evalFilterFloat64(data, cond.field, cond.operator, cond.value)
	default:
		val := filterFieldString(data, cond.field)
		return cmpString(val, cond.operator, cond.value)
	}
}

func evalFilterUint64(data map[string]any, field, op, expected string) bool {
	v := filterFieldUint64(data, field)
	e, err := strconv.ParseUint(expected, 10, 64)
	if err != nil {
		return false
	}
	switch op {
	case "exact":
		return v == e
	case "not_equal":
		return v != e
	case "gt":
		return v > e
	case "lt":
		return v < e
	case "gte":
		return v >= e
	case "lte":
		return v <= e
	default:
		return false
	}
}

func evalFilterFloat64(data map[string]any, field, op, expected string) bool {
	v := filterFieldFloat64(data, field)
	e, err := strconv.ParseFloat(expected, 64)
	if err != nil {
		return false
	}
	switch op {
	case "exact":
		return v-e < 1e-9 && e-v < 1e-9
	case "not_equal":
		return !(v-e < 1e-9 && e-v < 1e-9)
	case "gt":
		return v > e
	case "lt":
		return v < e
	case "gte":
		return v >= e
	case "lte":
		return v <= e
	default:
		return false
	}
}

func cmpString(actual, op, expected string) bool {
	switch op {
	case "exact":
		return actual == expected
	case "not_equal":
		return actual != expected
	case "contains":
		return strings.Contains(actual, expected)
	case "prefix":
		return strings.HasPrefix(actual, expected)
	case "suffix":
		return strings.HasSuffix(actual, expected)
	default:
		return false
	}
}

func filterFieldString(data map[string]any, field string) string {
	if v, ok := data[field]; ok {
		switch t := v.(type) {
		case string:
			return t
		default:
			return ""
		}
	}
	return ""
}

func filterFieldUint64(data map[string]any, field string) uint64 {
	if v, ok := data[field]; ok {
		switch t := v.(type) {
		case uint64:
			return t
		case uint32:
			return uint64(t)
		case int:
			return uint64(t)
		case int64:
			return uint64(t)
		case float64:
			return uint64(t)
		}
	}
	return 0
}

func filterFieldFloat64(data map[string]any, field string) float64 {
	if v, ok := data[field]; ok {
		switch t := v.(type) {
		case float64:
			return t
		case int:
			return float64(t)
		case int64:
			return float64(t)
		case uint64:
			return float64(t)
		}
	}
	return 0
}

// ── SSL data type detection ─────────────────────────────────────────
// DetectSSLDataType classifies SSL/TLS plaintext content into a high-level type.
// Mirrors AgentSight collector/src/analyzers/common.rs detect_data_type.

func DetectSSLDataType(data string) string {
	if data == "" {
		return "empty"
	}
	// HTTP detection
	if strings.HasPrefix(data, "HTTP/") {
		return "http_response"
	}
	for _, method := range []string{"GET ", "POST ", "PUT ", "DELETE ", "PATCH ", "HEAD ", "OPTIONS ", "CONNECT ", "TRACE "} {
		if strings.HasPrefix(data, method) {
			return "http_request"
		}
	}
	// SSE detection
	if strings.HasPrefix(data, "data:") || strings.HasPrefix(data, "event:") ||
		strings.HasPrefix(data, "id:") || strings.HasPrefix(data, "retry:") {
		return "sse"
	}
	// WebSocket detection (starts with HTTP upgrade)
	if strings.HasPrefix(data, "GET ") && strings.Contains(data, "Upgrade: websocket") {
		return "websocket"
	}
	// JSON detection
	trimmed := strings.TrimSpace(data)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		return "json"
	}
	// gRPC / protobuf — binary with specific header
	if len(data) >= 5 && data[0] == 0 && data[4]&0x80 == 0 {
		return "grpc"
	}
	// Binary heuristics
	if isBinaryData(data) {
		return "binary"
	}
	// Text
	return "text"
}

func isBinaryData(data string) bool {
	if len(data) == 0 {
		return false
	}
	binCount := 0
	maxCheck := len(data)
	if maxCheck > 128 {
		maxCheck = 128
	}
	for i := range maxCheck {
		if data[i] == 0 {
			return true
		}
		if data[i] < 0x09 || (data[i] > 0x0d && data[i] < 0x20) {
			binCount++
		}
	}
	return binCount > maxCheck/4
}
