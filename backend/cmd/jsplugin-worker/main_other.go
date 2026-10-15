//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "jsplugin-worker is supported on Linux only")
	os.Exit(1)
}
