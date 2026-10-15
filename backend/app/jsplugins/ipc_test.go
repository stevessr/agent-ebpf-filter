package jsplugins

import (
	"context"
	"os/exec"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testWorker(t *testing.T) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "worker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil { t.Fatal(err) }
	ctx, stop := context.WithCancel(context.Background())
	w := NewWorker()
	done := make(chan error, 1)
	go func(){ done <- w.Serve(ctx, listener) }()
	t.Cleanup(func(){
		stop()
		select {
		case err := <-done:
			if err != nil { t.Errorf("worker shut down: %v",err) }
		case <-time.After(time.Second):
			t.Error("worker did not stop")
		}
	})
	return socket
}

func TestWorkerIPCAndCache(t *testing.T) {
	socket := testWorker(t)
	source := `let count=0; export function onEvent(e){ return {count:++count, risk:e.risk}; }`
	for i:=0;i<3;i++{
		got,err := InvokeRemote(context.Background(),socket,source,"onEvent",json.RawMessage(`{"risk":88}`))
		if err!=nil || string(got)!=`{"count":1,"risk":88}` { t.Fatalf("invocation %d: %s, %v",i,got,err) }
	}
}

func TestWorkerFailClosedWithoutSocket(t *testing.T) {
	_, err := InvokeRemote(context.Background(), "", `export function onEvent(){return 1}`, "onEvent", json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(),"not configured"){ t.Fatalf("expected disabled, got %v",err) }
}

func TestWorkerRejectsMalformedAndOversizedFrames(t *testing.T) {
	socket := testWorker(t)
	for _, payload := range []string{"garbage\n",strings.Repeat("x",MaxWireBytes+2)+"\n",`{"version":999,"source":"x","hook":"onEvent","payload":{}}`+"\n"} {
		conn,err:=net.Dial("unix",socket)
		if err!=nil{t.Fatal(err)}
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		_,err=conn.Write([]byte(payload))
		if err != nil {
			conn.Close()
			continue
		}
		var out workerResponse
		err=json.NewDecoder(conn).Decode(&out)
		conn.Close()
		if err!=nil || out.Error==""{t.Fatalf("frame accepted: err=%v resp=%+v",err,out)}
	}
}

func TestWorkerRejectsRequestsAfterShutdown(t *testing.T) {
	dir:=t.TempDir()
	socket:=filepath.Join(dir,"gone.sock")
	listener,err:=net.Listen("unix",socket)
	if err!=nil { t.Fatal(err) }
	ctx,cancel:=context.WithCancel(context.Background())
	done:=make(chan error,1)
	go func(){done<-NewWorker().Serve(ctx,listener)}()
	cancel()
	if err:= <-done; err!=nil {t.Fatal(err)}
	_,err=InvokeRemote(context.Background(),socket,`export function onEvent(){return null}`,"onEvent",json.RawMessage(`{}`))
	if err==nil{t.Fatal("closed worker was callable")}
}

func TestWorkerConcurrentCalls(t *testing.T) {
	socket:=testWorker(t)
	var wg sync.WaitGroup
	for i:=0;i<12;i++ {
		wg.Add(1)
		go func(){
			defer wg.Done()
			_,err:=InvokeRemote(context.Background(),socket,`export function onEvent(e){return {ok:true}}`,"onEvent",json.RawMessage(`{}`))
			if err!=nil && !errors.Is(err,ErrExecutorBusy){t.Errorf("concurrent request: %v",err)}
		}()
	}
	wg.Wait()
}

func TestWorkerDeadlineAgainstHungExecution(t *testing.T) {
	socket:=testWorker(t)
	ctx,cancel:=context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_,err:=InvokeRemote(ctx,socket,`export function onEvent(){while(true){}}`,"onEvent",json.RawMessage(`{}`))
	if !errors.Is(err,context.DeadlineExceeded) {
		t.Fatalf("deadline not enforced: %v",err)
	}
}

func TestWorkerSocketPermissions(t *testing.T) {
	socket:=testWorker(t)
	if info,err:=os.Stat(socket);err!=nil || info.Mode()&os.ModeSocket==0 {
		t.Fatalf("missing worker socket: %v, %+v",err,info)
	}
}

func TestWorkerRemoteValidation(t *testing.T) {
	socket:=testWorker(t)
	if err:=ValidateRemote(context.Background(),socket,`export function onEvent(e){return e}`);err!=nil {
		t.Fatalf("valid source rejected: %v",err)
	}
	if err:=ValidateRemote(context.Background(),socket,`const invalid = ;`);err==nil {
		t.Fatal("worker accepted invalid JavaScript")
	}
	if err:=ValidateRemote(context.Background(),"",`export function onEvent(){return 1}`);err==nil {
		t.Fatal("validation silently fell back to backend")
	}
}

func TestWorkerRunsInSeparateOSProcess(t *testing.T) {
	socketEnv := "AGENT_EBPF_TEST_CHILD_SOCKET"
	socket := os.Getenv(socketEnv)
	if socket != "" {
		listener, err := net.Listen("unix", socket)
		if err != nil { t.Fatal(err) }
		if err := NewWorker().Serve(context.Background(), listener); err != nil { t.Fatal(err) }
		return
	}
	socket = filepath.Join(t.TempDir(), "subprocess.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorkerRunsInSeparateOSProcess$")
	cmd.Env = append(os.Environ(), socketEnv+"="+socket)
	if err:=cmd.Start();err!=nil { t.Fatal(err) }
	waited := false
	defer func(){ if !waited {cancel(); _ = cmd.Wait()} }()
	for i:=0;i<100;i++{
		if _,err:=os.Stat(socket);err==nil {break}
		select{
		case <-ctx.Done():
			t.Fatalf("worker never started: %v",ctx.Err())
		case <-time.After(10*time.Millisecond):
		}
	}
	out,err:=InvokeRemote(ctx,socket,`export function onEvent(e){return {separate:true, value:e.value}}`,"onEvent",json.RawMessage(`{"value":7}`))
	if err!=nil || string(out)!=`{"separate":true,"value":7}` {
		t.Fatalf("subprocess worker response=%s err=%v",out,err)
	}
	cancel()
	err = cmd.Wait()
	waited = true
	if err==nil {
		t.Fatal("worker subprocess should have been terminated by context")
	}
	// A terminated worker cannot be called again, regardless of an existing
	// listener socket inode from an unexpectedly interrupted subprocess.
	if _,err:=InvokeRemote(context.Background(),socket,`export function onEvent(){return 1}`,"onEvent",json.RawMessage(`{}`));err==nil {
		t.Fatal("terminated worker accepted a new call")
	}
}

func TestWorkerCancelsExecutionAfterClientDisconnect(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "cancellation.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil { t.Fatal(err) }
	ctx, stop := context.WithCancel(context.Background())
	worker := NewWorker()
	done := make(chan error, 1)
	go func() { done <- worker.Serve(ctx, listener) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil { t.Errorf("worker shutdown: %v", err) }
		case <-time.After(time.Second):
			t.Error("worker did not shut down")
		}
	})

	request := workerRequest{
		Version: ProtocolVersion, Operation: "invoke", Hook: "onEvent",
		Source: `export function onEvent(){while(true){}}`,
		Payload: json.RawMessage(`{}`),
	}
	conn, err := net.Dial("unix", socket)
	if err != nil { t.Fatal(err) }
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	// Wait for admission so the test doesn't pass just because the worker
	// has not accepted the request yet.
	deadline := time.Now().Add(time.Second)
	for len(worker.slots) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(worker.slots) == 0 {
		_ = conn.Close()
		t.Fatal("worker did not admit request")
	}
	_ = conn.Close()
	// Cancellation should release the execution slot promptly; without
	// disconnect handling it stays occupied until the 100ms VM deadline.
	released := false
	for i := 0; i < 35; i++ {
		if len(worker.slots) == 0 {
			released = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !released {
		t.Fatal("worker did not cancel disconnected invocation promptly")
	}
}

func TestWorkerEscapedSourceWithinConfiguredLimit(t *testing.T) {
	socket := testWorker(t)
	// encoding/json expands '<' to '\\u003c'. A valid source within the
	// 128 KiB plugin limit must survive this transport expansion.
	source := "//" + strings.Repeat("<", 70<<10) +
		"\nexport function onEvent(e) { return e; }"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ValidateRemote(ctx, socket, source); err != nil {
		t.Fatalf("escaped source rejected: %v", err)
	}
}

func TestWorkerReturnsTypedExecutionDeadline(t *testing.T) {
	socket := testWorker(t)
	// Keep the caller alive longer than the VM's own 100ms budget so the
	// error originates from the worker's versioned response, not a socket timeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := InvokeRemote(ctx, socket,
		`export function onEvent(){while(true){}}`,
		"onEvent", json.RawMessage(`{}`))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("worker execution deadline not preserved across IPC: %v", err)
	}
}
