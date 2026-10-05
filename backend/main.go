package main

import (
	"log"
	"os"

	"agent-ebpf-filter/app"
)

func main() {
	if err := app.ConfigureDesktopFlags(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
	if err := app.Main(); err != nil {
		log.Fatal(err)
	}
}
