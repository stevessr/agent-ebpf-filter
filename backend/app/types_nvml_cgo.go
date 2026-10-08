//go:build cgo

package app

import (
	"log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

func init() {
	if ret := nvml.Init(); ret == nvml.SUCCESS {
		nvmlInitialized = true
	} else {
		log.Printf("NVML Init failed: %v", nvml.ErrorString(ret))
	}
}
