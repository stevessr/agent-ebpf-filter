package app

import (
	"agent-ebpf-filter/pb"
	"agent-ebpf-filter/udsframe"
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

func TestDesktopFlagsSurviveElevation(t *testing.T) {
	if err := ConfigureDesktopFlags([]string{"--ebpf-bootstrap"}); err != nil {
		t.Fatal("legacy bootstrap flag rejected", err)
	}
	for _, key := range []string{"AGENT_DESKTOP_LIFETIME_SOCKET", "AGENT_FRONTEND_DIST", "AGENT_REAL_HOME", "AGENT_BACKEND_PORT", "GIN_MODE", "DISABLE_AUTH", "SUDO_ASKPASS"} {
		t.Setenv(key, "")
	}
	dir := t.TempDir()
	args := []string{"--desktop-lifetime-socket", filepath.Join(dir, "life.sock"), "--frontend-dir", filepath.Join(dir, "assets with spaces"), "--real-home", dir, "--desktop-port", "8091"}
	if err := ConfigureDesktopFlags(args); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GIN_MODE") != "release" || os.Getenv("DISABLE_AUTH") != "false" {
		t.Fatal("packaged desktop must enforce auth")
	}
	if os.Getenv("AGENT_BACKEND_PORT") != "8091" {
		t.Fatal("lost desktop port")
	}
	cmd := privilegedCommand("/usr/bin/pkexec", "/path with spaces/backend", args...)
	if !reflect.DeepEqual(cmd.Args[len(cmd.Args)-len(args):], args) {
		t.Fatal("desktop arguments lost across elevation")
	}
	t.Setenv("SUDO_ASKPASS", "/path with spaces/askpass")
	cmd = privilegedCommand("/usr/bin/sudo", "/backend", args...)
	if cmd.Args[1] != "-A" {
		t.Fatal("sudo must use configured askpass")
	}
	if err := ConfigureDesktopFlags(append(args, "--desktop-dev")); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GIN_MODE") != "debug" || os.Getenv("DISABLE_AUTH") != "true" {
		t.Fatal("dev mode not selected")
	}
}

func TestDesktopLifetimeEOFAndToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "life.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("AGENT_DESKTOP_LIFETIME_SOCKET", path)
	ctx, session, err := desktopSessionContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	done := make(chan error, 1)
	go func() { done <- session.publishToken("test-only-token") }()
	var message struct {
		Token            string `json:"token"`
		NativeIPCVersion int    `json:"nativeIpcVersion"`
	}
	if err := json.NewDecoder(conn).Decode(&message); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if message.Token != "test-only-token" {
		t.Fatal("token channel failed")
	}
	if message.NativeIPCVersion != desktopNativeIPCVersion {
		t.Fatalf("native IPC version = %d, want %d", message.NativeIPCVersion, desktopNativeIPCVersion)
	}
	if !session.publishProto(desktopFrameEventEnvelope, &pb.EventEnvelope{EventId: "evt_native_test"}) {
		t.Fatal("native frame was not queued")
	}
	frame, err := udsframe.Read(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(frame) < 2 || frame[0] != desktopFrameEventEnvelope {
		t.Fatalf("unexpected native frame: %v", frame)
	}
	var envelope pb.EventEnvelope
	if err := proto.Unmarshal(frame[1:], &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.GetEventId() != "evt_native_test" {
		t.Fatalf("native event id = %q", envelope.GetEventId())
	}
	_ = conn.Close()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("backend did not stop after desktop EOF")
	}
}

func TestDesktopStaticAssets(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("renew-bundle"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_FRONTEND_DIST", dir)
	router := gin.New()
	registerStaticRoutes(router)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/renew", nil))
	if response.Code != 200 || response.Body.String() != "renew-bundle" {
		t.Fatalf("bundled Renew route failed: %d", response.Code)
	}
}
