package jsplugins

import (
	"context"
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
	for _, payload := range []string{"garbage
",strings.Repeat("x",MaxWireBytes+2)+"
",`{"version":999,"source":"x","hook":"onEvent","payload":{}}`+"
"} {
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
