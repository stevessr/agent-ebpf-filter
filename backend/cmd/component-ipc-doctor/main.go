// Command component-ipc-doctor runs an isolated, rootless component IPC
// self-test. It neither contacts production sockets nor loads kernel programs.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"agent-ebpf-filter/internal/componentipc"
)

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type doctorReport struct {
	Status     string        `json:"status"`
	Scope      string        `json:"scope"`
	Checks     []checkResult `json:"checks"`
	DurationMS int64         `json:"duration_ms"`
	Error      string        `json:"error,omitempty"`
}

func (r *doctorReport) pass(name, detail string) {
	r.Checks = append(r.Checks, checkResult{Name: name, Status: "pass", Detail: detail})
}

func (r *doctorReport) fail(name string, err error) error {
	r.Checks = append(r.Checks, checkResult{Name: name, Status: "fail", Detail: err.Error()})
	r.Error = err.Error()
	return err
}

// runDoctor uses fresh in-memory credentials and one private, temporary UDS.
// It intentionally tests only this local machine, not a running deployment.
func runDoctor(ctx context.Context) (report doctorReport, err error) {
	start := time.Now()
	report = doctorReport{Status: "fail", Scope: "isolated_local_self_test", Checks: []checkResult{}}
	defer func() { report.DurationMS = time.Since(start).Milliseconds() }()
	if ctx == nil {
		err = report.fail("context", componentipc.ErrInvalidConfig)
		return report, err
	}
	if err := ctx.Err(); err != nil {
		err = report.fail("context", err)
		return report, err
	}
	if runtime.GOOS != "linux" {
		err = report.fail("platform", errors.New("SO_PEERCRED component IPC requires Linux"))
		return report, err
	}
	report.pass("platform", "Linux SO_PEERCRED available")
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()

	directory, err := os.MkdirTemp("", "agent-ebpf-ipc-doctor-")
	if err != nil {
		err = report.fail("private_directory", err)
		return report, err
	}
	defer os.RemoveAll(directory)
	directoryInfo, err := os.Stat(directory)
	if err != nil {
		err = report.fail("private_directory", err)
		return report, err
	}
	if directoryInfo.Mode().Perm() != 0700 {
		err = report.fail("private_directory", fmt.Errorf("directory permissions %04o, expected 0700", directoryInfo.Mode().Perm()))
		return report, err
	}
	report.pass("private_directory", "temporary 0700 directory")

	socketPath := filepath.Join(directory, "component.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		err = report.fail("private_socket", err)
		return report, err
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0600); err != nil {
		err = report.fail("private_socket", err)
		return report, err
	}
	socketInfo, err := os.Stat(socketPath)
	if err != nil {
		err = report.fail("private_socket", err)
		return report, err
	}
	if socketInfo.Mode()&os.ModeSocket == 0 {
		err = report.fail("private_socket", errors.New("listener did not create a Unix socket"))
		return report, err
	}
	if socketInfo.Mode().Perm() != 0600 {
		err = report.fail("private_socket", fmt.Errorf("socket permissions %04o, expected 0600", socketInfo.Mode().Perm()))
		return report, err
	}
	report.pass("private_socket", "owner-only 0600 Unix socket")

	// The secret never appears in args, logs, files or an environment variable.
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		err = report.fail("credentials", err)
		return report, err
	}
	stream, err := componentipc.NewStreamID()
	if err != nil {
		err = report.fail("credentials", err)
		return report, err
	}
	uid := uint32(os.Getuid())
	serverDone := make(chan error, 1)
	stopListener := context.AfterFunc(workCtx, func() { _ = listener.Close() })
	defer stopListener()

	go func() {
		raw, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		defer raw.Close()
		stopConn := context.AfterFunc(workCtx, func() { _ = raw.Close() })
		defer stopConn()
		server, handshakeErr := componentipc.HandshakeServer(raw, componentipc.ServerConfig{
			Secret: secret[:], Role: componentipc.RoleEngine,
			AllowedPeers: []componentipc.Role{componentipc.RoleCollector},
			VerifyPeer:   componentipc.VerifyUnixPeerUIDs(uid),
		})
		if handshakeErr != nil {
			serverDone <- handshakeErr
			return
		}
		defer server.Close()
		msg, readErr := server.ReceiveRecordCopy()
		if readErr != nil {
			serverDone <- readErr
			return
		}
		if msg.Kind != componentipc.KindEvent || msg.Data.Stream != stream ||
			msg.Data.Sequence != 1 || string(msg.Data.Payload) != "doctor-event" {
			serverDone <- errors.New("received record did not match the test event")
			return
		}
		receiver, makeErr := componentipc.NewSequenceReceiver(stream)
		if makeErr != nil {
			serverDone <- makeErr
			return
		}
		if checkErr := receiver.Check(msg.Data); checkErr != nil {
			serverDone <- checkErr
			return
		}
		receipt, commitErr := receiver.Commit(msg.Data)
		if commitErr != nil {
			serverDone <- commitErr
			return
		}
		serverDone <- server.SendAck(receipt)
	}()

	dialer := net.Dialer{}
	rawClient, err := dialer.DialContext(workCtx, "unix", socketPath)
	if err != nil {
		err = report.fail("peer_auth", err)
		return report, err
	}
	defer rawClient.Close()
	stopClient := context.AfterFunc(workCtx, func() { _ = rawClient.Close() })
	defer stopClient()
	client, err := componentipc.HandshakeClient(rawClient, componentipc.ClientConfig{
		Secret: secret[:], Role: componentipc.RoleCollector,
		ExpectedServer: componentipc.RoleEngine,
		VerifyPeer:     componentipc.VerifyUnixPeerUIDs(uid),
	})
	if err != nil {
		err = report.fail("peer_auth", err)
		return report, err
	}
	defer client.Close()
	report.pass("peer_auth", "bidirectional UID verification and HMAC challenge")

	tracker, err := componentipc.NewAckTracker(stream)
	if err != nil {
		err = report.fail("sequence_ack", err)
		return report, err
	}
	if err := client.SendSequenced(componentipc.KindEvent, componentipc.Sequenced{
		Stream: stream, Sequence: 1, Payload: []byte("doctor-event"),
	}); err != nil {
		err = report.fail("sequence_ack", err)
		return report, err
	}
	if err := tracker.MarkSent(1); err != nil {
		err = report.fail("sequence_ack", err)
		return report, err
	}
	record, err := client.ReceiveRecordCopy()
	if err != nil {
		err = report.fail("sequence_ack", err)
		return report, err
	}
	if record.Kind != componentipc.KindAck {
		err = report.fail("sequence_ack", errors.New("server did not return an ACK"))
		return report, err
	}
	if err := tracker.ApplyAck(record.Receipt); err != nil {
		err = report.fail("sequence_ack", err)
		return report, err
	}
	if tracker.LastAcked() != 1 {
		err = report.fail("sequence_ack", errors.New("ACK did not confirm sequence 1"))
		return report, err
	}
	select {
	case err := <-serverDone:
		if err != nil {
			err = report.fail("sequence_ack", err)
			return report, err
		}
	case <-workCtx.Done():
		err = report.fail("sequence_ack", workCtx.Err())
		return report, err
	}
	report.pass("sequence_ack", "versioned event and application ACK, sequence 1")
	report.Status = "pass"
	return report, nil
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("component-ipc-doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := flags.Bool("json", false, "print structured machine-readable diagnostics")
	timeout := flags.Duration("timeout", 8*time.Second, "deadline for isolated self-test")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *timeout <= 0 || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: component-ipc-doctor [--json] [--timeout=8s]")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	result, err := runDoctor(ctx)
	if *jsonOutput {
		if encodingErr := json.NewEncoder(stdout).Encode(result); encodingErr != nil {
			fmt.Fprintln(stderr, encodingErr)
			return 1
		}
	} else {
		for _, check := range result.Checks {
			fmt.Fprintf(stdout, "%s  %s: %s\n", check.Status, check.Name, check.Detail)
		}
		fmt.Fprintf(stdout, "Component IPC isolated self-test: %s (%d ms)\n", result.Status, result.DurationMS)
	}
	if err != nil {
		if !*jsonOutput {
			fmt.Fprintln(stderr, "IPC doctor:", err)
		}
		return 1
	}
	return 0
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}
