//go:build !linux

package sandbox

import "log"

func Apply() {
	log.Println("[SANDBOX] sandbox only available on Linux")
}
