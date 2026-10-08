//go:build cgo

package handlers

import (
	"path/filepath"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

func listCameraDevices() []string {
	matches, _ := filepath.Glob("/dev/video*")
	captureDevices := []string{}
	for _, dev := range matches {
		cam, err := device.Open(dev, device.WithIOType(v4l2.IOTypeMMAP))
		if err != nil {
			continue
		}
		if caps := cam.Capability(); caps.IsVideoCaptureSupported() {
			captureDevices = append(captureDevices, dev)
		}
		_ = cam.Close()
	}
	return captureDevices
}
