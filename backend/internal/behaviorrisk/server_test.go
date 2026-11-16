package behaviorrisk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPrivateServerEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "risk.sock")
	l, e := ListenPrivate(path)
	if e != nil {
		if errors.Is(e, syscall.EPERM) || errors.Is(e, syscall.EACCES) {
			t.Skip("environment denies Unix sockets; CI must execute this test")
		}
		t.Fatal(e)
	}
	b, h := encoded(validSpec())
	m := &Manager{}
	if e = m.Reload(bytes.NewReader(b), h); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Server{Manager: m}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, l) }()
	c, e := net.Dial("unix", path)
	if e != nil {
		if errors.Is(e, syscall.EPERM) || errors.Is(e, syscall.EACCES) {
			t.Skip("environment denies Unix sockets; CI must execute this test")
		}
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	for _, line := range []string{"{\"parameter\":\"good\"}\n", "{\"Parameter\":\"ambiguous\"}\n"} {
		if _, e = c.Write([]byte(line)); e != nil {
			t.Fatal(e)
		}
		raw, e := bufio.NewReader(c).ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		var r Result
		if e = json.Unmarshal(raw, &r); e != nil {
			t.Fatal(e)
		}
		if r.Mode != "shadow_only" {
			t.Fatal(r)
		}
		if line[2] == 'P' && r.Score != nil {
			t.Fatal("ambiguous request scored")
		}
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown blocked by idle client")
	}
	st := s.Stats()
	if st.Records != 2 || st.Abstained != 1 {
		t.Fatal(st)
	}
}
func TestSocketRefusesUnsafeDirectoryOrExistingFile(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0755)
	if _, e := ListenPrivate(filepath.Join(dir, "risk.sock")); e == nil {
		t.Fatal("unsafe directory accepted")
	}
	os.Chmod(dir, 0700)
	p := filepath.Join(dir, "keep")
	os.WriteFile(p, []byte("keep"), 0600)
	if _, e := ListenPrivate(p); e == nil {
		t.Fatal("overwrote existing path")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "keep" {
		t.Fatal("existing path changed")
	}
}
func TestConnectionLimit(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	l, e := ListenPrivate(filepath.Join(dir, "risk.sock"))
	if e != nil {
		if errors.Is(e, syscall.EPERM) || errors.Is(e, syscall.EACCES) {
			t.Skip("environment denies Unix sockets; CI must execute this test")
		}
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{Manager: &Manager{}}
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx, l) }()
	defer func() { cancel(); <-done }()
	var clients []net.Conn
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()
	for n := 0; n < MaxConnections+1; n++ {
		c, e := net.Dial("unix", l.Addr().String())
		if e != nil {
			t.Fatal(e)
		}
		clients = append(clients, c)
	}
	last := clients[len(clients)-1]
	last.SetReadDeadline(time.Now().Add(3 * time.Second))
	one := make([]byte, 1)
	_, e = last.Read(one)
	if e == nil {
		t.Fatal("excess connection accepted")
	}
	if s.Stats().Rejected != 1 {
		t.Fatal(s.Stats())
	}
}
