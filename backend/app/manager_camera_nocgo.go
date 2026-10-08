//go:build !cgo

package app

import (
	"context"
	"errors"
)

// Pure-Go desktop builds intentionally omit V4L2 camera capture because
// go4vl depends on CGO. The rest of the embedded backend remains available.
type CameraStream struct {
	devName string
}

type CameraSubscriber struct{}

func getCameraStream(devName string) *CameraStream {
	return &CameraStream{devName: devName}
}

func (s *CameraStream) Subscribe() *CameraSubscriber {
	return nil
}

func (sub *CameraSubscriber) NextFrame(context.Context) ([]byte, error) {
	return nil, errors.New("camera capture unavailable in pure-Go embedded backend")
}

func (sub *CameraSubscriber) Unsubscribe() {}

func shutdownCameraStreams(context.Context) error {
	return nil
}
