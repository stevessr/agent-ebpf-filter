package app

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/internal/behavior"
)

// ---- moved from backend/zz_merged_backend.go section dataset_parsers.go ----

const (
	remoteDatasetAbsoluteRecordLimit = 100_000
	remoteDatasetMaxRecordBytes      = 4 << 20
)

type remoteDatasetParseLimits struct {
	MaxRecords   int
	StoreRecords int
}

type remoteDatasetParseResult struct {
	Records   []remoteDatasetRecord
	Format    string
	Total     int
	Truncated bool
}

func normalizeRemoteDatasetParseLimits(limits remoteDatasetParseLimits) remoteDatasetParseLimits {
	if limits.MaxRecords <= 0 || limits.MaxRecords > remoteDatasetAbsoluteRecordLimit {
		limits.MaxRecords = remoteDatasetAbsoluteRecordLimit
	}
	if limits.StoreRecords < 0 || limits.StoreRecords > limits.MaxRecords {
		limits.StoreRecords = limits.MaxRecords
	}
	return limits
}

func appendLimitedRemoteDatasetRecord(result *remoteDatasetParseResult, record remoteDatasetRecord, limits remoteDatasetParseLimits) bool {
	if result.Total >= limits.MaxRecords {
		result.Truncated = true
		return false
	}
	result.Total++
	if len(result.Records) < limits.StoreRecords {
		result.Records = append(result.Records, record)
	}
	return true
}

func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Check first 1024 bytes for null bytes or excessive non-printable characters
	checkLen := len(data)
	if checkLen > 1024 {
		checkLen = 1024
	}
	nullCount := 0
	controlCount := 0
	for i := 0; i < checkLen; i++ {
		b := data[i]
		if b == 0 {
			nullCount++
		} else if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			controlCount++
		}
	}
	// Binary files almost always have nulls or many control characters.
	// ASCII/UTF-8 text files should not have nulls and very few control characters.
	return nullCount > 0 || controlCount > (checkLen/10)
}

func parseRemoteDatasetRecords(raw []byte, format string, source string) ([]remoteDatasetRecord, string, error) {
	result, err := parseRemoteDatasetRecordsWithLimits(raw, format, source, remoteDatasetParseLimits{
		MaxRecords:   remoteDatasetAbsoluteRecordLimit,
		StoreRecords: remoteDatasetAbsoluteRecordLimit,
	})
	return result.Records, result.Format, err
}

func parseRemoteDatasetRecordsWithLimits(raw []byte, format string, source string, limits remoteDatasetParseLimits) (remoteDatasetParseResult, error) {
	limits = normalizeRemoteDatasetParseLimits(limits)
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "auto"
	}

	// Early check for binary data if format is auto or text
	if (format == "auto" || format == "text" || format == "txt") && isBinary(raw) {
		// If it's binary but we're here, it means it wasn't recognized as an archive
		// or it's a corrupted archive. We should NOT treat it as text.
		return remoteDatasetParseResult{}, errors.New("unsupported binary data format; expected JSON, CSV, TSV or plain text")
	}

	switch format {
	case "json":
		return parseJSONDatasetRecordsWithLimits(raw, source, limits)
	case "jsonl", "ndjson":
		return parseJSONLinesDatasetRecordsWithLimits(raw, source, limits)
	case "csv":
		return parseDelimitedDatasetRecordsWithLimits(raw, ',', source, limits)
	case "tsv":
		return parseDelimitedDatasetRecordsWithLimits(raw, '\t', source, limits)
	case "text", "txt":
		return parseTextDatasetRecordsWithLimits(raw, source, limits)
	case "auto":
		if looksLikeJSON(raw) {
			if result, err := parseJSONDatasetRecordsWithLimits(raw, source, limits); err == nil {
				return result, nil
			}
			if result, err := parseJSONLinesDatasetRecordsWithLimits(raw, source, limits); err == nil && result.Total > 0 {
				return result, nil
			}
		}
		if looksLikeDelimited(raw) {
			if result, err := parseDelimitedDatasetRecordsWithLimits(raw, ',', source, limits); err == nil && result.Total > 0 {
				return result, nil
			}
			if result, err := parseDelimitedDatasetRecordsWithLimits(raw, '\t', source, limits); err == nil && result.Total > 0 {
				return result, nil
			}
		}
		return parseTextDatasetRecordsWithLimits(raw, source, limits)
	default:
		return remoteDatasetParseResult{}, fmt.Errorf("unsupported dataset format %q", format)
	}
}

func looksLikeJSON(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.HasPrefix(trimmed, []byte("{")) || bytes.HasPrefix(trimmed, []byte("["))
}

func looksLikeDelimited(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	firstLine := trimmed
	if idx := bytes.IndexByte(trimmed, '\n'); idx >= 0 {
		firstLine = trimmed[:idx]
	}
	return bytes.Contains(firstLine, []byte(",")) || bytes.Contains(firstLine, []byte("\t"))
}

func parseJSONDatasetRecords(raw []byte, source string) ([]remoteDatasetRecord, string, error) {
	result, err := parseJSONDatasetRecordsWithLimits(raw, source, remoteDatasetParseLimits{MaxRecords: remoteDatasetAbsoluteRecordLimit, StoreRecords: remoteDatasetAbsoluteRecordLimit})
	return result.Records, result.Format, err
}

func parseJSONDatasetRecordsWithLimits(raw []byte, source string, limits remoteDatasetParseLimits) (remoteDatasetParseResult, error) {
	result := remoteDatasetParseResult{Format: "json"}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return result, nil
	}

	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return remoteDatasetParseResult{}, err
	}

	rowIndex := 0
	visitDatasetJSONCandidates(decoded, func(item any) bool {
		rowIndex++
		record, ok := remoteDatasetRecordFromAny(item, rowIndex, source)
		if !ok {
			return true
		}
		return appendLimitedRemoteDatasetRecord(&result, record, limits)
	})
	return result, nil
}

func parseJSONLinesDatasetRecords(raw []byte, source string) ([]remoteDatasetRecord, string, error) {
	result, err := parseJSONLinesDatasetRecordsWithLimits(raw, source, remoteDatasetParseLimits{MaxRecords: remoteDatasetAbsoluteRecordLimit, StoreRecords: remoteDatasetAbsoluteRecordLimit})
	return result.Records, result.Format, err
}

func parseJSONLinesDatasetRecordsWithLimits(raw []byte, source string, limits remoteDatasetParseLimits) (remoteDatasetParseResult, error) {
	result := remoteDatasetParseResult{Format: "jsonl"}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), remoteDatasetMaxRecordBytes)
	rowIndex := 0
	for scanner.Scan() {
		rowIndex++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decoded any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			continue
		}
		record, ok := remoteDatasetRecordFromAny(decoded, rowIndex, source)
		if !ok {
			continue
		}
		if !appendLimitedRemoteDatasetRecord(&result, record, limits) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return remoteDatasetParseResult{}, fmt.Errorf("scan JSONL dataset: %w", err)
	}
	return result, nil
}

func parseDelimitedDatasetRecords(raw []byte, comma rune, source string) ([]remoteDatasetRecord, string, error) {
	result, err := parseDelimitedDatasetRecordsWithLimits(raw, comma, source, remoteDatasetParseLimits{MaxRecords: remoteDatasetAbsoluteRecordLimit, StoreRecords: remoteDatasetAbsoluteRecordLimit})
	return result.Records, result.Format, err
}

func parseDelimitedDatasetRecordsWithLimits(raw []byte, comma rune, source string, limits remoteDatasetParseLimits) (remoteDatasetParseResult, error) {
	format := "csv"
	if comma == '\t' {
		format = "tsv"
	}
	result := remoteDatasetParseResult{Format: format}
	reader := csv.NewReader(bytes.NewReader(raw))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	headerRow, err := reader.Read()
	if errors.Is(err, io.EOF) {
		result.Format = ""
		return result, nil
	}
	if err != nil {
		return remoteDatasetParseResult{}, err
	}

	header := normalizeHeaderRow(headerRow)
	if len(header) == 0 {
		header = make([]string, len(headerRow))
		for i := range header {
			header[i] = fmt.Sprintf("column_%d", i)
		}
	}

	rowIndex := 1
	for {
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return remoteDatasetParseResult{}, readErr
		}
		rowIndex++
		rowMap := make(map[string]any, len(header))
		for j, cell := range row {
			if j < len(header) {
				rowMap[header[j]] = strings.TrimSpace(cell)
			}
		}
		record, ok := remoteDatasetRecordFromMap(rowMap, rowIndex, source)
		if !ok {
			continue
		}
		if !appendLimitedRemoteDatasetRecord(&result, record, limits) {
			break
		}
	}
	return result, nil
}

func parseTextDatasetRecords(raw []byte, source string) []remoteDatasetRecord {
	result, _ := parseTextDatasetRecordsWithLimits(raw, source, remoteDatasetParseLimits{MaxRecords: remoteDatasetAbsoluteRecordLimit, StoreRecords: remoteDatasetAbsoluteRecordLimit})
	return result.Records
}

func parseTextDatasetRecordsWithLimits(raw []byte, source string, limits remoteDatasetParseLimits) (remoteDatasetParseResult, error) {
	result := remoteDatasetParseResult{Format: "text"}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), remoteDatasetMaxRecordBytes)
	pendingSELinuxRule := ""
	pendingSELinuxRow := 0
	rowIndex := 0
	for scanner.Scan() {
		rowIndex++
		line := strings.TrimSpace(scanner.Text())
		if shouldSkipTextDatasetLine(line) {
			continue
		}

		if pendingSELinuxRule != "" {
			if fragment := normalizeSELinuxPolicyRuleLine(line); fragment != "" {
				pendingSELinuxRule = strings.TrimSpace(pendingSELinuxRule + " " + fragment)
				if len(pendingSELinuxRule) > remoteDatasetMaxRecordBytes {
					return remoteDatasetParseResult{}, fmt.Errorf("SELinux dataset record exceeds %d bytes", remoteDatasetMaxRecordBytes)
				}
			}
			if selinuxPolicyStatementComplete(line) {
				if record, ok := selinuxPolicyRuleRecordFromLine(pendingSELinuxRule, pendingSELinuxRow, source); ok {
					if !appendLimitedRemoteDatasetRecord(&result, record, limits) {
						break
					}
				}
				pendingSELinuxRule = ""
				pendingSELinuxRow = 0
			}
			continue
		}

		if cleanedRule := normalizeSELinuxPolicyRuleLine(line); cleanedRule != "" {
			if _, ok := selinuxPolicyRuleLabel(cleanedRule); ok {
				if !selinuxPolicyStatementComplete(line) && strings.Contains(cleanedRule, "{") {
					pendingSELinuxRule = cleanedRule
					pendingSELinuxRow = rowIndex
					continue
				}
				if record, ok := selinuxPolicyRuleRecordFromLine(cleanedRule, rowIndex, source); ok {
					if !appendLimitedRemoteDatasetRecord(&result, record, limits) {
						break
					}
					continue
				}
			}
		}

		parts := splitCommandLine(line)
		if len(parts) == 0 {
			continue
		}
		allNumeric := true
		for _, part := range parts {
			if _, err := strconv.Atoi(part); err != nil {
				allNumeric = false
				break
			}
		}
		record := remoteDatasetRecord{Row: rowIndex, Source: source}
		record.CommandLine = line
		if allNumeric {
			if len(parts) == 1 {
				continue
			}
			record.Comm = "syscall-seq"
			record.Args = append([]string(nil), parts...)
		} else {
			record.Comm, record.Args = normalizeCommandInput(line, "", nil)
		}
		if record.Comm == "" {
			continue
		}
		record.UserLabel = "remote-import"
		if !appendLimitedRemoteDatasetRecord(&result, record, limits) {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return remoteDatasetParseResult{}, fmt.Errorf("scan text dataset: %w", err)
	}
	if pendingSELinuxRule != "" && !result.Truncated {
		if record, ok := selinuxPolicyRuleRecordFromLine(pendingSELinuxRule, pendingSELinuxRow, source); ok {
			appendLimitedRemoteDatasetRecord(&result, record, limits)
		}
	}
	return result, nil
}

func shouldSkipTextDatasetLine(line string) bool {
	if line == "" {
		return true
	}

	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(trimmed, "#"),
		strings.HasPrefix(trimmed, "//"),
		strings.HasPrefix(trimmed, "/*"),
		strings.HasPrefix(trimmed, "*/"),
		strings.HasPrefix(trimmed, "*"),
		strings.HasPrefix(lower, "__syscall("),
		strings.HasPrefix(lower, "#include"),
		strings.HasPrefix(lower, "#define"),
		strings.HasPrefix(lower, "#pragma"),
		strings.HasPrefix(lower, "typedef "),
		strings.HasPrefix(lower, "struct "),
		strings.HasPrefix(lower, "enum "),
		strings.HasPrefix(lower, "union "),
		strings.HasPrefix(lower, "static "),
		strings.HasPrefix(lower, "extern "):
		return true
	}

	return false
}

func flattenDatasetJSON(decoded any) []any {
	items, _ := flattenDatasetJSONWithLimit(decoded, remoteDatasetAbsoluteRecordLimit+1)
	return items
}

func flattenDatasetJSONWithLimit(decoded any, maxItems int) ([]any, bool) {
	if maxItems <= 0 {
		maxItems = 1
	}
	items := make([]any, 0, min(maxItems, 256))
	truncated := visitDatasetJSONCandidates(decoded, func(item any) bool {
		if len(items) >= maxItems {
			return false
		}
		items = append(items, item)
		return true
	})
	return items, truncated
}

// visitDatasetJSONCandidates walks supported dataset shapes without building a
// second expanded copy of the entire decoded JSON document. It returns true
// when the visitor stops traversal early.
func visitDatasetJSONCandidates(decoded any, visit func(any) bool) bool {
	visitItems := func(items []any) bool {
		for _, item := range items {
			if visitExpandedDatasetJSONItem(item, visit) {
				return true
			}
		}
		return false
	}

	switch value := decoded.(type) {
	case []any:
		return visitItems(value)
	case map[string]any:
		for _, key := range []string{"rows", "records", "items", "samples", "data", "commands", "rules", "executables"} {
			nested, ok := value[key]
			if !ok {
				continue
			}
			switch nestedValue := nested.(type) {
			case []any:
				return visitItems(nestedValue)
			case map[string]any:
				visitedObject := false
				for _, nestedKey := range sortedDatasetMapKeys(nestedValue) {
					nestedObject, ok := nestedValue[nestedKey].(map[string]any)
					if !ok {
						continue
					}
					visitedObject = true
					candidate := cloneDatasetMap(nestedObject, "")
					candidate["_injected_name"] = nestedKey
					if visitExpandedDatasetJSONItem(candidate, visit) {
						return true
					}
				}
				if !visitedObject {
					return visitExpandedDatasetJSONItem(nestedValue, visit)
				}
				return false
			}
		}

		allObjects := len(value) > 0
		for _, nested := range value {
			if _, ok := nested.(map[string]any); !ok {
				allObjects = false
				break
			}
		}
		if !allObjects {
			return visitExpandedDatasetJSONItem(value, visit)
		}
		for _, key := range sortedDatasetMapKeys(value) {
			if key == "functions" || key == "metadata" || key == "categories" || key == "contexts" {
				continue
			}
			candidate := cloneDatasetMap(value[key].(map[string]any), "")
			candidate["_injected_name"] = key
			if visitExpandedDatasetJSONItem(candidate, visit) {
				return true
			}
		}
		return false
	default:
		return !visit(decoded)
	}
}

func visitExpandedDatasetJSONItem(item any, visit func(any) bool) bool {
	object, ok := item.(map[string]any)
	if !ok {
		return !visit(item)
	}
	if functions, ok := object["functions"].(map[string]any); ok {
		for _, functionName := range sortedDatasetMapKeys(functions) {
			entries, ok := functions[functionName].([]any)
			if !ok {
				continue
			}
			for _, entry := range entries {
				entryObject, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				candidate := cloneDatasetMap(object, "functions")
				mergeDatasetMap(candidate, entryObject)
				candidate["_injected_category"] = functionName
				if !visit(candidate) {
					return true
				}
			}
		}
		return false
	}
	if commands, ok := object["Commands"].([]any); ok {
		for _, command := range commands {
			commandObject, ok := command.(map[string]any)
			if !ok {
				continue
			}
			candidate := cloneDatasetMap(object, "Commands")
			mergeDatasetMap(candidate, commandObject)
			if !visit(candidate) {
				return true
			}
		}
		return false
	}
	return !visit(object)
}

func cloneDatasetMap(value map[string]any, skipKey string) map[string]any {
	clone := make(map[string]any, len(value))
	for key, item := range value {
		if key != skipKey {
			clone[key] = item
		}
	}
	return clone
}

func mergeDatasetMap(dst, src map[string]any) {
	for key, item := range src {
		dst[key] = item
	}
}

func sortedDatasetMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func remoteDatasetRecordFromAny(decoded any, rowIndex int, source string) (remoteDatasetRecord, bool) {
	switch value := decoded.(type) {
	case string:
		if record, ok := selinuxPolicyRuleRecordFromLine(value, rowIndex, source); ok {
			return record, true
		}
		comm, args := normalizeCommandInput(value, "", nil)
		if comm == "" {
			return remoteDatasetRecord{}, false
		}
		return remoteDatasetRecord{
			Row:         rowIndex,
			Source:      source,
			CommandLine: value,
			Comm:        comm,
			Args:        args,
			UserLabel:   "remote-import",
		}, true
	case map[string]any:
		return remoteDatasetRecordFromMap(value, rowIndex, source)
	default:
		return remoteDatasetRecord{}, false
	}
}

func remoteDatasetRecordFromMap(row map[string]any, rowIndex int, source string) (remoteDatasetRecord, bool) {
	record := remoteDatasetRecord{Row: rowIndex, Source: source, UserLabel: "remote-import"}

	commandLine := firstStringValue(row, "commandLine", "commandline", "cmdline", "full_command", "command", "shell", "payload", "text", "value", "Command", "code", "rule", "policy", "selinuxRule", "selinuxrule", "selinux_rule")
	if selinuxRecord, ok := selinuxPolicyRuleRecordFromLine(commandLine, rowIndex, source); ok {
		label := normalizeDatasetLabelValue(row["label"])
		if label == "" {
			label = normalizeDatasetLabelValue(row["action"])
		}
		if label == "" {
			label = normalizeDatasetLabelValue(row["class"])
		}
		if label != "" {
			selinuxRecord.Label = label
			selinuxRecord.LabelSource = "dataset"
		}
		if category := firstStringValue(row, "category", "behavior", "type", "group", "Category", "_injected_category"); category != "" {
			selinuxRecord.Category = category
		}
		if anomaly, ok := extractDatasetFloat(row, "anomalyScore", "anomaly_score", "score", "riskScore"); ok {
			selinuxRecord.Anomaly = anomaly
			selinuxRecord.HasAnomaly = true
		}
		if ts, ok := extractDatasetTimestamp(row); ok {
			selinuxRecord.Timestamp = ts
		}
		if userLabel := firstStringValue(row, "userLabel", "userlabel", "user_label"); userLabel != "" {
			selinuxRecord.UserLabel = userLabel
		}
		return selinuxRecord, true
	}
	comm := firstStringValue(row, "comm", "commandName", "commandname", "name", "executable", "Name", "_injected_name")
	args := extractDatasetArgs(row, commandLine)
	if commandLine == "" && comm != "" {
		commandLine = joinCommandLine(comm, args)
	}
	if commandLine != "" && comm == "" {
		comm, args = normalizeCommandInput(commandLine, "", nil)
	}
	if comm == "" && commandLine == "" {
		return remoteDatasetRecord{}, false
	}

	record.CommandLine = commandLine
	record.Comm = comm
	record.Args = args
	record.Label = normalizeDatasetLabelValue(row["label"])
	if record.Label == "" {
		record.Label = normalizeDatasetLabelValue(row["action"])
	}
	if record.Label == "" {
		record.Label = normalizeDatasetLabelValue(row["class"])
	}
	if record.Label != "" {
		record.LabelSource = "dataset"
	}

	record.Category = firstStringValue(row, "category", "behavior", "type", "group", "Category", "_injected_category")
	if anomaly, ok := extractDatasetFloat(row, "anomalyScore", "anomaly_score", "score", "riskScore"); ok {
		record.Anomaly = anomaly
		record.HasAnomaly = true
	}
	if ts, ok := extractDatasetTimestamp(row); ok {
		record.Timestamp = ts
	}
	if userLabel := firstStringValue(row, "userLabel", "userlabel", "user_label"); userLabel != "" {
		record.UserLabel = userLabel
	}

	return record, true
}

func buildRemoteDatasetRow(record remoteDatasetRecord, mode string, cleanSensitive bool) remoteDatasetRow {
	if cleanSensitive {
		record = sanitizeRemoteDatasetRecord(record)
	}
	comm, args := normalizeCommandInput(record.CommandLine, record.Comm, record.Args)
	label := record.Label
	labelSource := record.LabelSource
	if label == "" {
		if inferredLabel, inferredSource := inferRemoteDatasetLabelFromSource(record.Source); inferredLabel != "" && strings.EqualFold(strings.TrimSpace(mode), "preserve") {
			label = inferredLabel
			labelSource = inferredSource
		}
	}
	if label == "" {
		label = "-"
	}
	if strings.EqualFold(strings.TrimSpace(mode), "block") {
		label = "BLOCK"
		labelSource = "forced"
	} else if strings.EqualFold(strings.TrimSpace(mode), "unlabeled") {
		label = "-"
		labelSource = "forced"
	} else if labelSource == "" {
		labelSource = "inferred"
	}

	category := record.Category
	if category == "" {
		category = behavior.ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := record.Anomaly
	if !record.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	timestamp := record.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	commandLine := strings.TrimSpace(record.CommandLine)
	if commandLine == "" {
		commandLine = joinCommandLine(comm, args)
	}

	return remoteDatasetRow{
		Row:          record.Row,
		Source:       record.Source,
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         args,
		Label:        label,
		LabelSource:  labelSource,
		Category:     category,
		AnomalyScore: anomaly,
		HasAnomaly:   record.HasAnomaly,
		Timestamp:    timestamp.Format(time.RFC3339),
		UserLabel:    record.UserLabel,
	}
}

func buildRemoteDatasetSample(row remoteDatasetRow, mode string, cleanSensitive bool) TrainingSample {
	if cleanSensitive {
		row = sanitizeRemoteDatasetRow(row)
	}
	comm, args := normalizeCommandInput(row.CommandLine, row.Comm, row.Args)
	timestamp := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, row.Timestamp); err == nil {
		timestamp = parsed.UTC()
	}

	category := row.Category
	if category == "" {
		category = behavior.ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := row.AnomalyScore
	if !row.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	label := int32(-1)
	userLabel := strings.TrimSpace(row.UserLabel)
	if userLabel == "" {
		userLabel = "remote-import"
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "block":
		label = actionFromLabel("BLOCK")
		userLabel = "remote-block"
	case "unlabeled":
		userLabel = "remote-import-unlabeled"
	default:
		if normalized := normalizeActionLabel(row.Label); normalized != "" {
			label = actionFromLabel(normalized)
			if userLabel == "remote-import" {
				userLabel = "remote-source-label"
			}
		} else if inferredLabel, inferredSource := inferRemoteDatasetLabelFromSource(row.Source); inferredLabel != "" {
			label = actionFromLabel(inferredLabel)
			if inferredSource != "" && userLabel == "remote-import" {
				userLabel = "remote-source-label"
			}
		} else if strings.EqualFold(strings.TrimSpace(mode), "heuristic") {
			assessment := assessCommandSafetyWithOptions(nil, comm, args, "", 0, commandSafetyAssessmentOptions{IncludeLLM: false})
			if action, ok := assessment["recommendedAction"].(string); ok {
				label = actionFromLabel(action)
				if userLabel == "remote-import" {
					userLabel = "remote-heuristic"
				}
			}
		}
	}

	features := globalFeatureExtractor.Extract(comm, args, "", 0)
	commandLine := strings.TrimSpace(row.CommandLine)
	if commandLine == "" {
		commandLine = joinCommandLine(comm, args)
	}
	return TrainingSample{
		Features:     features,
		Label:        label,
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         args,
		Category:     category,
		AnomalyScore: anomaly,
		Timestamp:    timestamp,
		UserLabel:    userLabel,
	}
}
