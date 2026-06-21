package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run test_rustls_offset.go <codex_binary_path>")
		os.Exit(1)
	}

	binPath := os.Args[1]
	fmt.Printf("Testing rustls offset detection for: %s\n", binPath)
	fmt.Println("=" + string(make([]byte, 60)))

	offsets, err := FindRustlsOffsets(binPath)
	if err != nil {
		log.Printf("Error finding rustls offsets: %v", err)
		os.Exit(1)
	}

	fmt.Printf("✓ rustls functions located:\n")
	if offsets.WriteTLS > 0 {
		fmt.Printf("  - write_tls: 0x%x\n", offsets.WriteTLS)
	} else {
		fmt.Printf("  - write_tls: not found\n")
	}

	if offsets.ReadTLS > 0 {
		fmt.Printf("  - read_tls:  0x%x\n", offsets.ReadTLS)
	} else {
		fmt.Printf("  - read_tls:  not found\n")
	}

	if offsets.WriteTLS == 0 && offsets.ReadTLS == 0 {
		fmt.Println("\n⚠ No rustls functions found. This binary may not use rustls.")
		os.Exit(1)
	}

	fmt.Println("\n✓ Ready for uprobe attachment")
}
