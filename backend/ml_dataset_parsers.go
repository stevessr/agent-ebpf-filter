package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "auto"
	}

	// Early check for binary data if format is auto or text
	if (format == "auto" || format == "text" || format == "txt") && isBinary(raw) {
		// If it's binary but we're here, it means it wasn't recognized as an archive
		// or it's a corrupted archive. We should NOT treat it as text.
		return nil, "", errors.New("unsupported binary data format; expected JSON, CSV, TSV or plain text")
	}

	switch format {
	case "json":
		return parseJSONDatasetRecords(raw, source)
	case "jsonl", "ndjson":
		return parseJSONLinesDatasetRecords(raw, source)
	case "csv":
		return parseDelimitedDatasetRecords(raw, ',', source)
	case "tsv":
		return parseDelimitedDatasetRecords(raw, '\t', source)
	case "text", "txt":
		return parseTextDatasetRecords(raw, source), "text", nil
	case "auto":
		if looksLikeJSON(raw) {
			if records, detected, err := parseJSONDatasetRecords(raw, source); err == nil {
				return records, detected, nil
			}
			if records, detected, err := parseJSONLinesDatasetRecords(raw, source); err == nil && len(records) > 0 {
				return records, detected, nil
			}
		}
		if looksLikeDelimited(raw) {
			if records, detected, err := parseDelimitedDatasetRecords(raw, ',', source); err == nil && len(records) > 0 {
				return records, detected, nil
			}
			if records, detected, err := parseDelimitedDatasetRecords(raw, '\t', source); err == nil && len(records) > 0 {
				return records, detected, nil
			}
		}
		return parseTextDatasetRecords(raw, source), "text", nil
	default:
		return nil, "", fmt.Errorf("unsupported dataset format %q", format)
	}
}

func looksLikeJSON(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func looksLikeDelimited(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return false
	}
	firstLine := trimmed
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		firstLine = trimmed[:idx]
	}
	return strings.Contains(firstLine, ",") || strings.Contains(firstLine, "\t")
}

func parseJSONDatasetRecords(raw []byte, source string) ([]remoteDatasetRecord, string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, "json", nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, "", err
	}

	items := flattenDatasetJSON(decoded)
	if len(items) == 0 {
		return nil, "json", nil
	}
	records := make([]remoteDatasetRecord, 0, len(items))
	for i, item := range items {
		record, ok := remoteDatasetRecordFromAny(item, i+1, source)
		if !ok {
			continue
		}
		records = append(records, record)
	}
	return records, "json", nil
}

func parseJSONLinesDatasetRecords(raw []byte, source string) ([]remoteDatasetRecord, string, error) {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	records := make([]remoteDatasetRecord, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var decoded any
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			continue
		}
		record, ok := remoteDatasetRecordFromAny(decoded, i+1, source)
		if !ok {
			continue
		}
		records = append(records, record)
	}
	return records, "jsonl", nil
}

func parseDelimitedDatasetRecords(raw []byte, comma rune, source string) ([]remoteDatasetRecord, string, error) {
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", nil
	}

	header := normalizeHeaderRow(rows[0])
	if len(header) == 0 {
		header = make([]string, len(rows[0]))
		for i := range header {
			header[i] = fmt.Sprintf("column_%d", i)
		}
	}

	records := make([]remoteDatasetRecord, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		rowMap := make(map[string]any, len(header))
		for j, cell := range rows[i] {
			if j < len(header) {
				rowMap[header[j]] = strings.TrimSpace(cell)
			}
		}
		record, ok := remoteDatasetRecordFromMap(rowMap, i+1, source)
		if !ok {
			continue
		}
		records = append(records, record)
	}

	format := "csv"
	if comma == '\t' {
		format = "tsv"
	}
	return records, format, nil
}

func parseTextDatasetRecords(raw []byte, source string) []remoteDatasetRecord {
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	records := make([]remoteDatasetRecord, 0, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if shouldSkipTextDatasetLine(line) {
			continue
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
		record := remoteDatasetRecord{Row: i + 1, Source: source}
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
		records = append(records, record)
	}
	return records
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
	var items []any
	switch value := decoded.(type) {
	case []any:
		items = value
	case map[string]any:
		found := false
	outer:
		for _, key := range []string{"rows", "records", "items", "samples", "data", "commands", "rules", "executables"} {
			if nested, ok := value[key]; ok {
				switch nestedValue := nested.(type) {
				case []any:
					items = nestedValue
					found = true
					break outer
				case map[string]any:
					if expanded := expandDatasetObjectMap(nestedValue); len(expanded) > 0 {
						items = expanded
					} else {
						items = []any{nestedValue}
					}
					found = true
					break outer
				}
			}
		}
		if !found {
			// Check if it's a map of objects (GTFOBins style)
			allObjects := true
			for _, v := range value {
				if _, ok := v.(map[string]any); !ok {
					allObjects = false
					break
				}
			}
			if allObjects && len(value) > 0 {
				for k, v := range value {
					// Skip known metadata keys at the top level
					if k == "functions" || k == "metadata" || k == "categories" || k == "contexts" {
						continue
					}
					m := v.(map[string]any)
					m["_injected_name"] = k
					items = append(items, m)
				}
			} else {
				items = []any{value}
			}
		}
	default:
		return []any{decoded}
	}

	// Second pass: expand nested commands (GTFOBins 'functions' or LOLBAS 'Commands')
	var expanded []any
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			expanded = append(expanded, item)
			continue
		}

		// GTFOBins expansion
		if funcs, ok := m["functions"].(map[string]any); ok {
			for fName, fList := range funcs {
				if fl, ok := fList.([]any); ok {
					for _, fi := range fl {
						if fim, ok := fi.(map[string]any); ok {
							newM := make(map[string]any)
							for k, v := range m { // copy original
								if k != "functions" {
									newM[k] = v
								}
							}
							for k, v := range fim { // merge function entry
								newM[k] = v
							}
							newM["_injected_category"] = fName
							expanded = append(expanded, newM)
						}
					}
				}
			}
			continue
		}

		// LOLBAS expansion
		if cmds, ok := m["Commands"].([]any); ok {
			for _, ci := range cmds {
				if cim, ok := ci.(map[string]any); ok {
					newM := make(map[string]any)
					for k, v := range m { // copy original
						if k != "Commands" {
							newM[k] = v
						}
					}
					for k, v := range cim { // merge command entry
						newM[k] = v
					}
					expanded = append(expanded, newM)
				}
			}
			continue
		}

		expanded = append(expanded, m)
	}

	return expanded
}

func expandDatasetObjectMap(value map[string]any) []any {
	items := make([]any, 0, len(value))
	for k, v := range value {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		m["_injected_name"] = k
		items = append(items, m)
	}
	return items
}

func remoteDatasetRecordFromAny(decoded any, rowIndex int, source string) (remoteDatasetRecord, bool) {
	switch value := decoded.(type) {
	case string:
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

	commandLine := firstStringValue(row, "commandLine", "cmdline", "full_command", "command", "shell", "payload", "text", "value", "Command", "code")
	comm := firstStringValue(row, "comm", "commandName", "name", "executable", "Name", "_injected_name")
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
	if userLabel := firstStringValue(row, "userLabel", "user_label"); userLabel != "" {
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
	} else if labelSource == "" {
		labelSource = "inferred"
	}

	category := record.Category
	if category == "" {
		category = ClassifyBehavior(comm, args).PrimaryCategory
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
		category = ClassifyBehavior(comm, args).PrimaryCategory
	}
	anomaly := row.AnomalyScore
	if !row.HasAnomaly {
		_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		anomaly = globalEmbedder.ComputeAnomalyScore(emb)
	}

	label := int32(-1)
	userLabel := "remote-import"
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "block":
		label = actionFromLabel("BLOCK")
		userLabel = "remote-block"
	case "unlabeled":
		userLabel = "remote-import-unlabeled"
	default:
		if normalized := normalizeActionLabel(row.Label); normalized != "" {
			label = actionFromLabel(normalized)
			userLabel = "remote-source-label"
		} else if inferredLabel, inferredSource := inferRemoteDatasetLabelFromSource(row.Source); inferredLabel != "" {
			label = actionFromLabel(inferredLabel)
			if inferredSource != "" {
				userLabel = "remote-source-label"
			}
		} else if strings.EqualFold(strings.TrimSpace(mode), "heuristic") {
			assessment := assessCommandSafety(context.Background(), comm, args, "", 0)
			if action, ok := assessment["recommendedAction"].(string); ok {
				label = actionFromLabel(action)
				userLabel = "remote-heuristic"
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

func normalizeDatasetLabelValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return normalizeActionLabel(v)
	case json.Number:
		if n, err := strconv.Atoi(v.String()); err == nil {
			return actionLabel[int32(n)]
		}
	case float64:
		return actionLabel[int32(v)]
	case int:
		return actionLabel[int32(v)]
	case int64:
		return actionLabel[int32(v)]
	case uint32:
		return actionLabel[int32(v)]
	case uint64:
		return actionLabel[int32(v)]
	}
	return ""
}

func normalizeActionLabel(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "0", "ALLOW", "BENIGN", "SAFE", "NORMAL", "NORM", "PASSED", "PASS":
		return "ALLOW"
	case "1", "BLOCK", "DENY", "REJECT", "MALICIOUS", "MALWARE", "BAD", "ANOM", "ANOMALY", "ATTACK", "INTRUSION", "CMDI", "COMMAND INJECTION", "SQLI", "SQL INJECTION", "XSS", "CROSS-SITE SCRIPTING", "PATH-TRAVERSAL", "PATH_TRAVERSAL", "PATH TRAVERSAL":
		return "BLOCK"
	case "2", "REWRITE", "TRANSFORM", "MODIFY":
		return "REWRITE"
	case "3", "ALERT", "WARN", "WARNING", "SUSPICIOUS":
		return "ALERT"
	default:
		return ""
	}
}

func extractDatasetArgs(row map[string]any, commandLine string) []string {
	if args := extractDatasetStringSlice(row, "args", "argv", "arguments", "commandArgs"); len(args) > 0 {
		return args
	}
	if raw := firstAnyValue(row, "args", "argv", "arguments", "commandArgs"); raw != nil {
		if str, ok := raw.(string); ok && strings.TrimSpace(str) != "" {
			return splitCommandLine(str)
		}
	}
	if commandLine != "" {
		_, args := normalizeCommandInput(commandLine, "", nil)
		return args
	}
	return nil
}

func extractDatasetStringSlice(row map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := row[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case []any:
			out := make([]string, 0, len(value))
			for _, item := range value {
				if s := fmt.Sprint(item); strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if strings.TrimSpace(value) != "" {
				return splitCommandLine(value)
			}
		}
	}
	return nil
}

func extractDatasetFloat(row map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := row[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case float64:
			return value, true
		case float32:
			return float64(value), true
		case int:
			return float64(value), true
		case int64:
			return float64(value), true
		case json.Number:
			if f, err := value.Float64(); err == nil {
				return f, true
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func extractDatasetTimestamp(row map[string]any) (time.Time, bool) {
	raw := firstAnyValue(row, "timestamp", "time", "createdAt", "created_at", "ts")
	if raw == nil {
		return time.Time{}, false
	}
	switch value := raw.(type) {
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			if ts, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
				return ts, true
			}
		}
		if num, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parseUnixTimestamp(num), true
		}
	case float64:
		return parseUnixTimestamp(int64(value)), true
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return parseUnixTimestamp(n), true
		}
	case int64:
		return parseUnixTimestamp(value), true
	case int:
		return parseUnixTimestamp(int64(value)), true
	}
	return time.Time{}, false
}

func parseUnixTimestamp(v int64) time.Time {
	switch {
	case v > 1_000_000_000_000:
		return time.Unix(0, v*int64(time.Millisecond)).UTC()
	case v > 1_000_000_000:
		return time.Unix(v, 0).UTC()
	default:
		return time.Unix(v, 0).UTC()
	}
}

func firstStringValue(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := row[key]; ok && raw != nil {
			switch v := raw.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case map[string]any, []any:
				// Special serialization: if it's an object/array, return as JSON string
				b, _ := json.Marshal(v)
				return string(b)
			default:
				s := fmt.Sprint(v)
				if strings.TrimSpace(s) != "" && s != "<nil>" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func firstAnyValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if raw, ok := row[key]; ok {
			if raw != nil {
				return raw
			}
		}
	}
	return nil
}

func normalizeHeaderRow(headers []string) []string {
	out := make([]string, 0, len(headers))
	for _, header := range headers {
		header = strings.ToLower(strings.TrimSpace(header))
		header = strings.ReplaceAll(header, " ", "")
		header = strings.ReplaceAll(header, "-", "_")
		if header != "" {
			out = append(out, header)
		} else {
			out = append(out, "")
		}
	}
	return out
}
