package main

import (
	"fmt"
	"sort"

	"agent-ebpf-filter/backend/app"
)

func main() {
	ds, err := app.LoadClassicDataset("iris")
	if err != nil {
		panic(err)
	}

	rows := len(ds.Features)
	cols := 0
	if rows > 0 {
		cols = len(ds.Features[0])
	}

	counts := make(map[string]int)
	for _, label := range ds.Labels {
		counts[label]++
	}

	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("samples=%d features=%d\n", rows, cols)
	fmt.Printf("class_distribution=")
	for i, k := range keys {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf("%s:%d", k, counts[k])
	}
	fmt.Println()

	limit := 5
	if rows < limit {
		limit = rows
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("sample_%d features=%v label=%s\n", i, ds.Features[i], ds.Labels[i])
	}
}
