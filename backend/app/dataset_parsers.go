package app

import (
	"agent-ebpf-filter/internal/behavior"
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

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

		parts := behavior.SplitCommandLine(line)
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
			record.Comm, record.Args = behavior.NormalizeCommandInput(line, "", nil)
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
