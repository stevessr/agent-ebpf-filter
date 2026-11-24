package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

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
