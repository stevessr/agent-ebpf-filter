//go:build linux

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"agent-ebpf-filter/app/jsplugins"
)

func TestWorkerBinaryLifecycle(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("non-root worker lifecycle test; root refusal is checked separately")
	}
	binary := filepath.Join(t.TempDir(), "jsplugin-worker")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build worker: %v\n%s", err, output)
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil { t.Fatal(err) }
	socket := filepath.Join(root, "worker.sock")
	cmd := exec.Command(binary, "--socket", socket)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil { t.Fatal(err) }
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	// Check readiness via actual IPC, not merely creation of a socket inode.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	source := `export function onEvent(e) { return { verified: true, count: e.count }; }`
	var result json.RawMessage
	var callErr error
	for {
		result, callErr = jsplugins.InvokeRemote(ctx, socket, source, "onEvent", json.RawMessage(`{"count":9}`))
		if callErr == nil { break }
		if ctx.Err() != nil {
			t.Fatalf("real worker startup failed: %v, stderr=%s", callErr, stderr.String())
		}
		time.Sleep(15 * time.Millisecond)
	}
	if string(result) != `{"verified":true,"count":9}` {
		t.Fatalf("unexpected worker result: %s", result)
	}
	info, err := os.Stat(socket)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("worker socket mode: info=%v, err=%v", info, err)
	}

	// A second worker must not take over an already-bound private socket.
	second := exec.Command(binary, "--socket", socket)
	if out, err := second.CombinedOutput(); err == nil || !strings.Contains(string(out), "already exists") {
		t.Fatalf("second worker unexpectedly accepted socket: %v %s", err, out)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil { t.Fatal(err) }
	waitResult := make(chan error, 1)
	go func() { waitResult <- cmd.Wait() }()
	select {
	case err := <-waitResult:
		waited = true
		if err != nil { t.Fatalf("worker graceful termination: %v, stderr=%s", err, stderr.String()) }
	case <-time.After(3*time.Second):
		t.Fatal("worker ignored SIGTERM")
	}
	if _, err := os.Lstat(socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("worker left stale socket on shutdown: %v", err)
	}
	if _, err := jsplugins.InvokeRemote(context.Background(), socket, source, "onEvent", json.RawMessage(`{}`)); err == nil {
		t.Fatal("stopped worker remained callable")
	}
}

func TestWorkerBinaryRequiresPrivateSocketDirectory(t *testing.T) {
	if os.Geteuid() == 0 { t.Skip("non-root worker test") }
	binary := filepath.Join(t.TempDir(), "jsplugin-worker")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil { t.Fatalf("build worker: %v %s",err,out) }
	shared := filepath.Join(t.TempDir(), "shared")
	if err:=os.Mkdir(shared,0o755);err!=nil {t.Fatal(err)}
	if err:=os.Chmod(shared,0o755);err!=nil {t.Fatal(err)}
	socket := filepath.Join(shared,"worker.sock")
	cmd := exec.Command(binary,"--socket",socket)
	if out,err:=cmd.CombinedOutput();err==nil || !strings.Contains(string(out),"private 0700") {
		t.Fatalf("shared directory unexpectedly accepted: %v %s",err,out)
	}
	if _,err:=os.Lstat(socket);!errors.Is(err,os.ErrNotExist) {
		t.Fatalf("unauthorized socket was created: %v",err)
	}

	private:=filepath.Join(t.TempDir(),"real-private")
	if err:=os.Mkdir(private,0o700);err!=nil {t.Fatal(err)}
	link:=filepath.Join(t.TempDir(),"dir-symlink")
	if err:=os.Symlink(private,link);err!=nil {t.Fatal(err)}
	cmd=exec.Command(binary,"--socket",filepath.Join(link,"worker.sock"))
	if out,err:=cmd.CombinedOutput();err==nil || !strings.Contains(string(out),"private 0700") {
		t.Fatalf("symlink directory unexpectedly accepted: %v %s",err,out)
	}
}

func TestWorkerBinaryRecoversAfterSIGKILL(t *testing.T) {
	if os.Geteuid() == 0 { t.Skip("non-root worker test") }
	binary := filepath.Join(t.TempDir(), "jsplugin-worker")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build worker: %v %s", err, out)
	}
	private := filepath.Join(t.TempDir(), "private")
	if err:=os.Mkdir(private,0o700);err!=nil{t.Fatal(err)}
	socket := filepath.Join(private, "worker.sock")
	start := func() *exec.Cmd {
		t.Helper()
		cmd := exec.Command(binary, "--socket", socket)
		if err:=cmd.Start();err!=nil{t.Fatal(err)}
		t.Cleanup(func(){ if cmd.ProcessState==nil { _=cmd.Process.Kill(); _=cmd.Wait() } })
		ctx, cancel:=context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		for {
			if _, err:=jsplugins.ValidateRemote(ctx,socket,`export function onEvent(e){return e}`);err==nil{ return cmd }
			if ctx.Err()!=nil {t.Fatalf("worker never became ready: %v",ctx.Err())}
			time.Sleep(10*time.Millisecond)
		}
	}
	first:=start()
	if err:=first.Process.Kill();err!=nil{t.Fatal(err)}
	_ = first.Wait()
	if _,err:=os.Lstat(socket);err!=nil {t.Fatalf("expected stale socket after SIGKILL: %v",err)}
	second:=start()
	result,err:=jsplugins.InvokeRemote(context.Background(),socket,
		`export function onEvent(){return {restarted:true}}`,
		"onEvent",json.RawMessage(`{}`))
	if err!=nil || string(result)!=`{"restarted":true}` {
		t.Fatalf("restarted worker: %s, %v", result,err)
	}
	if err:=second.Process.Signal(syscall.SIGTERM);err!=nil{t.Fatal(err)}
	if err:=second.Wait();err!=nil{t.Fatal(err)}
}
