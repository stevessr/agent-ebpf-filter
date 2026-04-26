package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

type CameraStream struct {
	devName    string
	cam        *device.Device
	listeners  map[chan []byte]bool
	mu         sync.Mutex
	stopTimer  *time.Timer
	cancelFunc context.CancelFunc
	running    bool
}

var (
	activeStreams = make(map[string]*CameraStream)
	streamsMu     sync.Mutex
)

func getCameraStream(devName string) (*CameraStream, chan []byte) {
	streamsMu.Lock()
	s, ok := activeStreams[devName]
	if !ok {
		s = &CameraStream{
			devName:   devName,
			listeners: make(map[chan []byte]bool),
		}
		activeStreams[devName] = s
	}
	streamsMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// If there was a scheduled stop, cancel it
	if s.stopTimer != nil {
		s.stopTimer.Stop()
		s.stopTimer = nil
	}

	ch := make(chan []byte, 10)
	s.listeners[ch] = true

	if !s.running {
		var cam *device.Device
		var err error
		
		// Retry mechanism for Device Busy
		for i := 0; i < 3; i++ {
			cam, err = device.Open(devName, device.WithIOType(v4l2.IOTypeMMAP))
			if err == nil {
				break
			}
			log.Printf("[WARN] camera open retry %d for %s: %v", i+1, devName, err)
			time.Sleep(500 * time.Millisecond)
		}
		
		if err != nil {
			log.Printf("[ERROR] failed to open camera %s: %v", devName, err)
			delete(s.listeners, ch)
			return nil, nil
		}

		// Try MJPEG first, then fallback or fail
		if err := cam.SetPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtMJPEG, Width: 640, Height: 480}); err != nil {
			log.Printf("[WARN] MJPEG not supported for %s, trying default: %v", devName, err)
			// Some devices might only support YUYV or others, but our frontend expects JPEG
			// For now, we just fail gracefully if MJPEG (which most webcams have) fails
			cam.Close()
			delete(s.listeners, ch)
			return nil, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		if err := cam.Start(ctx); err != nil {
			log.Printf("[ERROR] failed to start camera %s: %v", devName, err)
			cancel()
			cam.Close()
			delete(s.listeners, ch)
			return nil, nil
		}

		s.cam = cam
		s.cancelFunc = cancel
		s.running = true

		go func(stream *CameraStream) {
			output := stream.cam.GetOutput()
			for frame := range output {
				stream.mu.Lock()
				for listener := range stream.listeners {
					select {
					case listener <- frame:
					default:
					}
				}
				stream.mu.Unlock()
			}
			
			// Physical cleanup only happens here
			stream.mu.Lock()
			if stream.cam != nil {
				_ = stream.cam.Stop()
				_ = stream.cam.Close()
				stream.cam = nil
			}
			stream.running = false
			stream.mu.Unlock()

			streamsMu.Lock()
			delete(activeStreams, stream.devName)
			streamsMu.Unlock()
		}(s)
	}

	return s, ch
}

func (s *CameraStream) unregister(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.listeners, ch)

	// If no one is listening, wait for a grace period before stopping
	if len(s.listeners) == 0 && s.running {
		if s.stopTimer != nil {
			s.stopTimer.Stop()
		}
		s.stopTimer = time.AfterFunc(5*time.Second, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.listeners) == 0 && s.running {
				if s.cancelFunc != nil {
					s.cancelFunc()
					s.cancelFunc = nil
				}
			}
		})
	}
}
