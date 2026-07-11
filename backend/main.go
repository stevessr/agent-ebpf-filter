package main

import (
	"log"

	"agent-ebpf-filter/app"
)

func main() {
	if err := app.Main(); err != nil {
		log.Fatal(err)
	}
}
