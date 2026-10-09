package main

import (
	"fmt"
	"sort"
	"strings"
)

// Render one bounded page of leaf fields. The previous fixed 300-row output
// made records with large payloads impossible to inspect without raw JSON.
// Keep scanning separate from rendering and never allocate the whole tree.
const (
	eventFieldPageSize = 60
	eventFieldScanLimit = 30000
	eventFieldDepthLimit = 18
)

type eventDetailFieldPage struct {
	Items []eventDetailField
	HasMore bool
	ScanLimited bool
	Scanned int
}

func eventDetailTreePage(detail map[string]any, query string, offset, limit int) eventDetailFieldPage {
	var page eventDetailFieldPage
	if detail == nil || limit <= 0 {
		return page
	}
	if offset < 0 {
		offset = 0
	}
	if limit > 300 {
		limit = 300
	}
	query = strings.ToLower(strings.TrimSpace(query))
	matched := 0
	stop := false
	var walk func(string, any, int)
	walk = func(path string, value any, depth int) {
		if stop { return }
		page.Scanned++
		if page.Scanned > eventFieldScanLimit {
			page.ScanLimited = true
			stop = true
			return
		}
		if depth > eventFieldDepthLimit {
			page.ScanLimited = true
			return
		}
		switch node := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(node))
			for key := range node { keys = append(keys, key) }
			sort.Strings(keys)
			for _, key := range keys {
				child := path+"."+key
				if path == "$" { child = key }
				walk(child, node[key], depth+1)
				if stop { return }
			}
		case []any:
			for index, item := range node {
				walk(fmt.Sprintf("%s[%d]", path, index), item, depth+1)
				if stop { return }
			}
		default:
			if value == nil { return }
			text := detailScalarText(value)
			if query != "" && !strings.Contains(strings.ToLower(path), query) &&
				!strings.Contains(strings.ToLower(text), query) {
				return
			}
			if matched >= offset+limit {
				page.HasMore = true
				stop = true
				return
			}
			if matched >= offset {
				page.Items = append(page.Items, eventDetailField{Label: path, Value: text})
			}
			matched++
		}
	}
	walk("$", detail, 0)
	if page.ScanLimited {
		// A bounded scan cannot establish that the omitted portion is empty.
		page.HasMore = true
	}
	return page
}

// Kept as a convenience for existing field-browser test fixtures.
func eventDetailTreeItems(detail map[string]any, query string) []eventDetailField {
	return eventDetailTreePage(detail, query, 0, 300).Items
}
