//go:build !linux

package main

import "log"

func ApplySandbox() {
	log.Println("[SANDBOX] sandbox only available on Linux")
}
