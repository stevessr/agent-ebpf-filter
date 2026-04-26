package main

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

type CameraStream struct {
	devName string
	cam     *device.Device

	// Zero-copy broadcasting mechanism
	latestFrame []byte
	frameCond   *sync.Cond
	frameMu     sync.Mutex // Changed from RWMutex to Mutex to fix sync.Cond panic

	subscriberCount int32
	stopTimer       *time.Timer
	cancelFunc      context.CancelFunc
	running         bool
	streamMu        sync.Mutex
}

var (
	activeStreams = make(map[string]*CameraStream)
	streamsMu     sync.Mutex
)

// Consumer representation
type CameraSubscriber struct {
	stream *CameraStream
	closed int32
}

func getCameraStream(devName string) *CameraStream {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	s, ok := activeStreams[devName]
	if !ok {
		s = &CameraStream{
			devName: devName,
		}
		s.frameCond = sync.NewCond(&s.frameMu)
		activeStreams[devName] = s
	}
	return s
}

func (s *CameraStream) Subscribe() *CameraSubscriber {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	atomic.AddInt32(&s.subscriberCount, 1)

	// Cancel any pending shutdown
	if s.stopTimer != nil {
		s.stopTimer.Stop()
		s.stopTimer = nil
	}

	if !s.running {
		var cam *device.Device
		var err error
		
		for i := 0; i < 3; i++ {
			cam, err = device.Open(s.devName, device.WithIOType(v4l2.IOTypeMMAP))
			if err == nil {
				break
			}
			log.Printf("[WARN] camera open retry %d for %s: %v", i+1, s.devName, err)
			time.Sleep(500 * time.Millisecond)
		}

		if err != nil {
			log.Printf("[ERROR] failed to open camera %s: %v", s.devName, err)
			atomic.AddInt32(&s.subscriberCount, -1)
			return nil
		}

		if err := cam.SetPixFormat(v4l2.PixFormat{PixelFormat: v4l2.PixelFmtMJPEG, Width: 640, Height: 480}); err != nil {
			log.Printf("[WARN] MJPEG not supported on %s: %v", s.devName, err)
			cam.Close()
			atomic.AddInt32(&s.subscriberCount, -1)
			return nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		if err := cam.Start(ctx); err != nil {
			log.Printf("[ERROR] failed to start camera stream %s: %v", s.devName, err)
			cancel()
			cam.Close()
			atomic.AddInt32(&s.subscriberCount, -1)
			return nil
		}

		s.cam = cam
		s.cancelFunc = cancel
		s.running = true

		// Independent producer thread
		go func(stream *CameraStream) {
			output := stream.cam.GetOutput()
			for frame := range output {
				stream.frameMu.Lock()
				stream.latestFrame = frame 
				stream.frameMu.Unlock()
				stream.frameCond.Broadcast()
			}

			// Hardware cleanup
			stream.streamMu.Lock()
			if stream.cam != nil {
				_ = stream.cam.Stop()
				_ = stream.cam.Close()
				stream.cam = nil
			}
			stream.running = false
			stream.streamMu.Unlock()

			streamsMu.Lock()
			delete(activeStreams, stream.devName)
			streamsMu.Unlock()
			
			stream.frameCond.Broadcast()
		}(s)
	}

	return &CameraSubscriber{stream: s}
}

func (sub *CameraSubscriber) NextFrame(ctx context.Context) ([]byte, error) {
	if atomic.LoadInt32(&sub.closed) == 1 {
		return nil, context.Canceled
	}
	
	s := sub.stream
	s.frameMu.Lock() // Must use Lock() for sync.Cond.Wait()
	s.frameCond.Wait()
	
	if !s.running || atomic.LoadInt32(&sub.closed) == 1 {
		s.frameMu.Unlock()
		return nil, context.Canceled
	}
	
	select {
	case <-ctx.Done():
		s.frameMu.Unlock()
		return nil, ctx.Err()
	default:
	}
	
	frame := s.latestFrame
	s.frameMu.Unlock()
	return frame, nil
}

func (sub *CameraSubscriber) Unsubscribe() {
	if !atomic.CompareAndSwapInt32(&sub.closed, 0, 1) {
		return 
	}

	s := sub.stream
	s.frameCond.Broadcast() 

	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	count := atomic.AddInt32(&s.subscriberCount, -1)
	if count <= 0 && s.running {
		if s.stopTimer != nil {
			s.stopTimer.Stop()
		}
		s.stopTimer = time.AfterFunc(5*time.Second, func() {
			s.streamMu.Lock()
			defer s.streamMu.Unlock()
			if atomic.LoadInt32(&s.subscriberCount) <= 0 && s.running {
				if s.cancelFunc != nil {
					s.cancelFunc()
					s.cancelFunc = nil
				}
			}
		})
	}
}
