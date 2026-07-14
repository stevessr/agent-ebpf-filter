package handlers

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"agent-ebpf-filter/app/wsstream"
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

var cameraDevicePattern = regexp.MustCompile(`^/dev/video[0-9]+$`)

func normalizeCameraDeviceName(raw string) (string, error) {
	deviceName := strings.TrimSpace(raw)
	if deviceName == "" {
		deviceName = "/dev/video0"
	}
	deviceName = filepath.Clean(deviceName)
	if !cameraDevicePattern.MatchString(deviceName) {
		return "", fmt.Errorf("invalid camera device %q", raw)
	}
	return deviceName, nil
}

func HandleCameraSnapshot(c *gin.Context) {
	devName, err := normalizeCameraDeviceName(c.Query("device"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	stream := Deps.GetCameraStream(devName)
	if stream == nil {
		c.JSON(500, gin.H{"error": "Failed to access camera"})
		return
	}
	sub := stream.Subscribe()
	if sub == nil {
		c.JSON(500, gin.H{"error": "Failed to access camera"})
		return
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
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
	out, _ := runBoundedHardwareCommand(c.Request.Context(), hardwareDiscoveryTimeout, hardwareDiscoveryMaxOutputBytes, "arecord", "-l")
	re := regexp.MustCompile(`card (\d+): .*? \[([^\]]+)\], device (\d+): .*? \[([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(string(out), -1)
	for _, m := range matches {
		if len(devices) >= hardwareDeviceListLimit {
			break
		}
		if len(m) >= 5 {
			id := fmt.Sprintf("hw:%s,%s", m[1], m[3])
			if seen[id] {
				continue
			}
			seen[id] = true
			devices = append(devices, gin.H{
				"id":          id,
				"name":        boundedHandlerText(fmt.Sprintf("%s (%s)", m[2], m[4]), hardwareDeviceLabelMaxBytes),
				"source_type": "alsa",
			})
		}
	}

	// 2. PulseAudio / PipeWire sources (includes Bluetooth A2DP/HFP)
	if pactlOut, err := runBoundedHardwareCommand(c.Request.Context(), hardwareDiscoveryTimeout, hardwareDiscoveryMaxOutputBytes, "pactl", "list", "sources", "short"); err == nil {
		// pactl output lines: <index>\t<name>\t<driver>\t<format>\t<state>
		for _, line := range strings.Split(strings.TrimSpace(string(pactlOut)), "\n") {
			if len(devices) >= hardwareDeviceListLimit {
				break
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			sourceName, err := normalizeMicrophoneDeviceName(fields[1])
			if err != nil || sourceName == "default" {
				continue
			}

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
				name = boundedHandlerText("🎧 [BT] "+sourceName, hardwareDeviceLabelMaxBytes)
			} else {
				name = boundedHandlerText("🎙 "+sourceName, hardwareDeviceLabelMaxBytes)
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
	device, err := normalizeMicrophoneDeviceName(c.Query("device"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	select {
	case microphoneCaptureSlots <- struct{}{}:
		defer func() { <-microphoneCaptureSlots }()
	default:
		c.JSON(429, gin.H{"error": "too many active microphone streams"})
		return
	}

	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsstream.ControlReadLimit)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	captureCtx, cancelCapture := context.WithCancel(c.Request.Context())
	defer cancelCapture()

	// Choose recording backend based on device type
	var cmd *exec.Cmd
	if strings.HasPrefix(device, "hw:") {
		// ALSA hardware device
		cmd = exec.CommandContext(captureCtx, "arecord", "-D", "plughw:"+device[3:], "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
	} else if device == "default" {
		// ALSA default (routes through PulseAudio/PipeWire if available)
		cmd = exec.CommandContext(captureCtx, "arecord", "-D", "default", "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
	} else {
		// PulseAudio / PipeWire source (Bluetooth, USB, etc.)
		// Try parec first, fall back to arecord with the source name
		if _, err := exec.LookPath("parec"); err == nil {
			cmd = exec.CommandContext(captureCtx, "parec", "-d", device, "--format=s16le", "--rate=16000", "--channels=1")
		} else {
			// Fallback: try arecord with the device name as-is
			cmd = exec.CommandContext(captureCtx, "arecord", "-D", device, "-f", "S16_LE", "-r", "16000", "-c", "1", "-t", "raw")
		}
	}
	cmd.WaitDelay = hardwareProcessWaitDelay

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		_ = wsstream.WriteMessage(conn, websocket.TextMessage, []byte("Error: Failed to open microphone device"))
		return
	}

	go func() {
		select {
		case <-done:
		case <-c.Request.Context().Done():
		}
		cancelCapture()
	}()

	buf := make([]byte, 4096)
	for {
		n, err := stdout.Read(buf)
		if err != nil || n == 0 {
			break
		}
		if err := wsstream.WriteMessage(conn, websocket.BinaryMessage, buf[:n]); err != nil {
			break
		}
	}
	cancelCapture()
	_ = cmd.Wait()
}

func ServeCameraWS(c *gin.Context) {
	devName, err := normalizeCameraDeviceName(c.Query("device"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsstream.ControlReadLimit)

	stream := Deps.GetCameraStream(devName)
	if stream == nil {
		_ = wsstream.WriteMessage(conn, websocket.TextMessage, []byte("Error: Failed to access camera"))
		return
	}
	sub := stream.Subscribe()
	if sub == nil {
		_ = wsstream.WriteMessage(conn, websocket.TextMessage, []byte("Error: Failed to access camera"))
		return
	}
	defer sub.Unsubscribe()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	go func() {
		defer cancel()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		frame, err := sub.NextFrame(ctx)
		if err != nil {
			return
		}
		if err := wsstream.WriteMessage(conn, websocket.BinaryMessage, frame); err != nil {
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
	conn.SetReadLimit(wsstream.ControlReadLimit)

	iv := wsstream.IntervalMilliseconds(c.Query("interval"), 2*time.Second, wsstream.MinStreamInterval, wsstream.MaxStreamInterval)
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
			data, err := proto.Marshal(snap)
			if err != nil {
				return
			}
			if err := wsstream.WriteMessage(conn, websocket.BinaryMessage, data); err != nil {
				return
			}
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}
