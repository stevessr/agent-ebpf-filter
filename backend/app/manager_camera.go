package app

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
)

// ---- moved from backend/zz_merged_backend.go section manager_camera.go ----

type CameraStream struct {
	devName string
	cam     *device.Device

	// Zero-copy broadcasting mechanism
	latestFrame []byte
	frameSeq    uint64
	frameNotify chan struct{}
	frameMu     sync.Mutex

	subscriberCount int32
	stopTimer       *time.Timer
	cancelFunc      context.CancelFunc
	producerDone    chan struct{}
	running         bool
	stopping        bool
	streamMu        sync.Mutex
}

var (
	activeStreams = make(map[string]*CameraStream)
	streamsMu     sync.Mutex
)

// Consumer representation
type CameraSubscriber struct {
	stream       *CameraStream
	done         chan struct{}
	nextMu       sync.Mutex
	lastFrameSeq uint64
	closed       int32
}

func getCameraStream(devName string) *CameraStream {
	streamsMu.Lock()
	defer streamsMu.Unlock()

	s, ok := activeStreams[devName]
	if !ok {
		s = &CameraStream{
			devName:     devName,
			frameNotify: make(chan struct{}),
		}
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

	// failSubscribe cleans up on failure: decrement refcount and remove
	// dead stream from activeStreams so subsequent attempts start fresh.
	failSubscribe := func() *CameraSubscriber {
		count := atomic.AddInt32(&s.subscriberCount, -1)
		if count <= 0 && !s.running {
			streamsMu.Lock()
			if activeStreams[s.devName] == s {
				delete(activeStreams, s.devName)
			}
			streamsMu.Unlock()
		}
		return nil
	}
	streamsMu.Lock()
	current, registered := activeStreams[s.devName]
	if !registered {
		activeStreams[s.devName] = s
	}
	streamsMu.Unlock()
	if registered && current != s {
		return failSubscribe()
	}
	if s.stopping {
		return failSubscribe()
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
			return failSubscribe()
		}

		// Try pixel formats in order: MJPEG → YUYV; fall back to device default
		formats := []v4l2.PixFormat{
			{PixelFormat: v4l2.PixelFmtMJPEG, Width: 640, Height: 480},
			{PixelFormat: v4l2.PixelFmtYUYV, Width: 640, Height: 480},
		}
		formatOK := false
		for _, f := range formats {
			if fmtErr := cam.SetPixFormat(f); fmtErr == nil {
				formatOK = true
				break
			} else {
				log.Printf("[WARN] pix format %#x not supported on %s: %v", f.PixelFormat, s.devName, fmtErr)
			}
		}
		if !formatOK {
			log.Printf("[WARN] explicit formats failed for %s, trying device default", s.devName)
		}

		ctx, cancel := context.WithCancel(context.Background())
		if err := cam.Start(ctx); err != nil {
			log.Printf("[ERROR] failed to start camera stream %s: %v", s.devName, err)
			cancel()
			cam.Close()
			return failSubscribe()
		}

		s.cam = cam
		s.cancelFunc = cancel
		s.producerDone = make(chan struct{})
		s.running = true
		s.stopping = false

		// Independent producer thread
		go func(stream *CameraStream, cam *device.Device, done chan struct{}) {
			defer close(done)
			output := cam.GetOutput()
			for frame := range output {
				stream.publishFrame(frame)
			}

			// Hardware cleanup
			stream.streamMu.Lock()
			if stream.cancelFunc != nil {
				stream.cancelFunc()
				stream.cancelFunc = nil
			}
			if stream.stopTimer != nil {
				stream.stopTimer.Stop()
				stream.stopTimer = nil
			}
			_ = cam.Stop()
			_ = cam.Close()
			if stream.cam == cam {
				stream.cam = nil
			}
			stream.running = false
			stream.stopping = false
			stream.streamMu.Unlock()
			stream.notifyFrameWaiters()
		}(s, cam, s.producerDone)
	}

	s.frameMu.Lock()
	lastFrameSeq := s.frameSeq
	s.frameMu.Unlock()
	return &CameraSubscriber{
		stream:       s,
		done:         make(chan struct{}),
		lastFrameSeq: lastFrameSeq,
	}
}

func (sub *CameraSubscriber) NextFrame(ctx context.Context) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if atomic.LoadInt32(&sub.closed) == 1 {
		return nil, context.Canceled
	}

	sub.nextMu.Lock()
	defer sub.nextMu.Unlock()

	s := sub.stream
	for {
		if atomic.LoadInt32(&sub.closed) == 1 {
			return nil, context.Canceled
		}

		s.frameMu.Lock()
		if s.frameSeq != sub.lastFrameSeq {
			sub.lastFrameSeq = s.frameSeq
			frame := s.latestFrame
			s.frameMu.Unlock()
			return frame, nil
		}
		notify := s.frameNotifyLocked()
		s.frameMu.Unlock()

		if !s.isRunning() {
			return nil, context.Canceled
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-sub.done:
			return nil, context.Canceled
		case <-notify:
		}
	}
}

func (sub *CameraSubscriber) Unsubscribe() {
	if !atomic.CompareAndSwapInt32(&sub.closed, 0, 1) {
		return
	}
	close(sub.done)

	s := sub.stream
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	count := atomic.AddInt32(&s.subscriberCount, -1)
	if count <= 0 && s.running {
		if s.stopTimer != nil {
			s.stopTimer.Stop()
		}
		var timer *time.Timer
		timer = time.AfterFunc(5*time.Second, func() {
			s.streamMu.Lock()
			defer s.streamMu.Unlock()
			if s.stopTimer != timer {
				return
			}
			s.stopTimer = nil
			if atomic.LoadInt32(&s.subscriberCount) <= 0 && s.running {
				s.stopping = true
				if s.cancelFunc != nil {
					s.cancelFunc()
				}
			}
		})
		s.stopTimer = timer
	}
}

func (s *CameraStream) publishFrame(frame []byte) {
	s.frameMu.Lock()
	s.latestFrame = frame
	s.frameSeq++
	notify := s.frameNotifyLocked()
	close(notify)
	s.frameNotify = make(chan struct{})
	s.frameMu.Unlock()
}

func (s *CameraStream) notifyFrameWaiters() {
	s.frameMu.Lock()
	notify := s.frameNotifyLocked()
	close(notify)
	s.frameNotify = make(chan struct{})
	s.frameMu.Unlock()
}

func (s *CameraStream) frameNotifyLocked() chan struct{} {
	if s.frameNotify == nil {
		s.frameNotify = make(chan struct{})
	}
	return s.frameNotify
}

func (s *CameraStream) isRunning() bool {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	return s.running
}

func shutdownCameraStreams(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	streamsMu.Lock()
	streams := make([]*CameraStream, 0, len(activeStreams))
	for _, stream := range activeStreams {
		streams = append(streams, stream)
	}
	streamsMu.Unlock()

	doneChannels := make([]<-chan struct{}, 0, len(streams))
	for _, stream := range streams {
		stream.streamMu.Lock()
		if stream.stopTimer != nil {
			stream.stopTimer.Stop()
			stream.stopTimer = nil
		}
		if stream.running {
			stream.stopping = true
			if stream.cancelFunc != nil {
				stream.cancelFunc()
			}
		}
		if stream.producerDone != nil {
			doneChannels = append(doneChannels, stream.producerDone)
		}
		stream.streamMu.Unlock()
		stream.notifyFrameWaiters()
	}

	for _, done := range doneChannels {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
