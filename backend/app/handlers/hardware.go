package handlers

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/vladimirvivien/go4vl/device"
	"github.com/vladimirvivien/go4vl/v4l2"
	"google.golang.org/protobuf/proto"
)

// ---- moved from app/handlershardwaresystem.go ----

func HandleSensors(c *gin.Context) {
	temps, _ := host.SensorsTemperatures()
	snap := &pb.SensorsSnapshot{Fans: []string{}}
	for _, t := range temps {
		snap.Temperatures = append(snap.Temperatures, &pb.SensorReading{
			Key:   t.SensorKey,
			Value: t.Temperature,
		})
	}
	Deps.WriteProtoOrJSON(c, 200, snap, gin.H{"temperatures": temps, "fans": []string{}})
}

func HandleCameras(c *gin.Context) {
	matches, _ := filepath.Glob("/dev/video*")
	captureDevices := []string{}
	for _, dev := range matches {
		cam, err := device.Open(dev, device.WithIOType(v4l2.IOTypeMMAP))
		if err == nil {
			if caps := cam.Capability(); caps.IsVideoCaptureSupported() {
				captureDevices = append(captureDevices, dev)
			}
			cam.Close()
		}
	}
	c.JSON(200, captureDevices)
}

func HandleCameraSnapshot(c *gin.Context) {
	devName := c.Query("device")
	if devName == "" {
		devName = "/dev/video0"
	}

	stream := Deps.GetCameraStream(devName)
	sub := stream.Subscribe()
	if sub == nil {
		c.JSON(500, gin.H{"error": "Failed to access camera"})
		return
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	frame, err := sub.NextFrame(ctx)
	if err != nil {
		c.JSON(500, gin.H{"error": "Timeout or error waiting for frame from camera"})
		return
	}
	c.Data(200, "image/jpeg", frame)
}

func HandleMicrophones(c *gin.Context) {
	cmd := exec.Command("arecord", "-l")
	out, _ := cmd.Output()
	re := regexp.MustCompile(`card (\d+): .*? \[([^\]]+)\], device (\d+): .*? \[([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	devices := []gin.H{}
	for _, m := range matches {
		if len(m) >= 5 {
			devices = append(devices, gin.H{"id": fmt.Sprintf("hw:%s,%s", m[1], m[3]), "name": fmt.Sprintf("%s (%s)", m[2], m[4])})
		}
	}
	if len(devices) == 0 {
		devices = append(devices, gin.H{"id": "default", "name": "Default Input"})
	}
	c.JSON(200, devices)
}

func ServeMicrophoneWS(c *gin.Context) {
	device := c.DefaultQuery("device", "default")
	if strings.HasPrefix(device, "hw:") {
		device = "plughw:" + device[3:]
	}
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	cmd := exec.Command("arecord", "-D", device, "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}

	go func() {
		select {
		case <-done:
		case <-c.Request.Context().Done():
		}
		_ = cmd.Process.Kill()
	}()

	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if err != nil || n == 0 {
			break
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
			break
		}
	}
	_ = cmd.Wait()
}

func ServeCameraWS(c *gin.Context) {
	devName := c.Query("device")
	if devName == "" {
		devName = "/dev/video0"
	}
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	stream := Deps.GetCameraStream(devName)
	sub := stream.Subscribe()
	if sub == nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Error: Failed to access camera"))
		return
	}
	defer sub.Unsubscribe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-done
		cancel()
	}()

	for {
		frame, err := sub.NextFrame(ctx)
		if err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			return
		}
	}
}

func ServeSensorsWS(c *gin.Context) {
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	intervalStr := c.DefaultQuery("interval", "2000")
	iv, _ := time.ParseDuration(intervalStr + "ms")
	if iv < 500*time.Millisecond {
		iv = 500 * time.Millisecond
	}
	ticker := time.NewTicker(iv)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			temps, _ := host.SensorsTemperatures()
			snap := &pb.SensorsSnapshot{Fans: []string{}}
			for _, t := range temps {
				snap.Temperatures = append(snap.Temperatures, &pb.SensorReading{
					Key:   t.SensorKey,
					Value: t.Temperature,
				})
			}
			data, _ := proto.Marshal(snap)
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}