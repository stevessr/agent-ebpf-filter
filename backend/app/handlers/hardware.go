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
	devices := []gin.H{}
	seen := make(map[string]bool)

	// 1. ALSA hardware capture devices (arecord -l)
	cmd := exec.Command("arecord", "-l")
	out, _ := cmd.Output()
	re := regexp.MustCompile(`card (\d+): .*? \[([^\]]+)\], device (\d+): .*? \[([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	for _, m := range matches {
		if len(m) >= 5 {
			id := fmt.Sprintf("hw:%s,%s", m[1], m[3])
			if seen[id] {
				continue
			}
			seen[id] = true
			devices = append(devices, gin.H{
				"id":          id,
				"name":        fmt.Sprintf("%s (%s)", m[2], m[4]),
				"source_type": "alsa",
			})
		}
	}

	// 2. PulseAudio / PipeWire sources (includes Bluetooth A2DP/HFP)
	if pactlOut, err := exec.Command("pactl", "list", "sources", "short").Output(); err == nil {
		// pactl output lines: <index>\t<name>\t<driver>\t<format>\t<state>
		for _, line := range strings.Split(strings.TrimSpace(string(pactlOut)), "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			sourceName := fields[1]

			// Skip monitor sources (output sinks, not input devices)
			if strings.HasSuffix(sourceName, ".monitor") {
				continue
			}

			if seen[sourceName] {
				continue
			}
			seen[sourceName] = true

			isBlueZ := strings.Contains(sourceName, "bluez") || strings.Contains(sourceName, "bluez_input")
			name := sourceName
			if isBlueZ {
				name = "🎧 [BT] " + sourceName
			} else {
				name = "🎙 " + sourceName
			}

			devices = append(devices, gin.H{
				"id":          sourceName,
				"name":        name,
				"source_type": "pulse",
			})
		}
	}

	if len(devices) == 0 {
		devices = append(devices, gin.H{"id": "default", "name": "Default Input", "source_type": "alsa"})
	}
	c.JSON(200, devices)
}

func ServeMicrophoneWS(c *gin.Context) {
	device := c.DefaultQuery("device", "default")

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

	// Choose recording backend based on device type
	var cmd *exec.Cmd
	if strings.HasPrefix(device, "hw:") {
		// ALSA hardware device
		cmd = exec.Command("arecord", "-D", "plughw:"+device[3:], "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
	} else if device == "default" {
		// ALSA default (routes through PulseAudio/PipeWire if available)
		cmd = exec.Command("arecord", "-D", "default", "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
	} else {
		// PulseAudio / PipeWire source (Bluetooth, USB, etc.)
		// Try parec first, fall back to arecord with the source name
		if _, err := exec.LookPath("parec"); err == nil {
			cmd = exec.Command("parec", "-d", device, "--format=s16le", "--rate=16000", "--channels=1")
		} else {
			// Fallback: try arecord with the device name as-is
			cmd = exec.Command("arecord", "-D", device, "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Error: Failed to open microphone device"))
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