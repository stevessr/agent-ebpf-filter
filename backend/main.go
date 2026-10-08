package main

import (
	"log"
	"os"

	"agent-ebpf-filter/embedded"
)

func main() {
	if err := embedded.Run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}
