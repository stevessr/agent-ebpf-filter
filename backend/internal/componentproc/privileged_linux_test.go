//go:build linux

package componentproc

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/internal/componentshm"
	"agent-ebpf-filter/internal/componentwire"
)

func TestPrivilegedSupervisorFailsClosedWithoutExplicitIdentity(t *testing.T) {
	publish := func(*componentipc.Session, *componentshm.Ring) error { return nil }
	if err := RunEngineSharedAsUID(nil, "/bin/true", 65534, 65534, 4, 512, publish); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("nil context accepted: %v", err)
	}
	if err := RunEngineSharedAsUID(context.Background(), "relative", 65534, 65534, 4, 512, publish); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("relative executable accepted: %v", err)
	}
	if os.Geteuid() != 0 {
		if err := RunEngineSharedAsUID(context.Background(), "/bin/true", 65534, 65534, 4, 512, publish); !errors.Is(err, ErrUnsafeSupervisor) {
			t.Fatalf("unprivileged caller attempted dropped-UID launch: %v", err)
		}
		return
	}
	for _, ids := range [][2]uint32{{0, 65534}, {65534, 0}} {
		if err := RunEngineSharedAsUID(context.Background(), "/bin/true", ids[0], ids[1], 4, 512, publish); !errors.Is(err, ErrInvalidEngineIdentity) {
			t.Fatalf("root or unspecified identity accepted: %v", err)
		}
	}
}

func TestPrivilegedSharedEngineDropsUID(t *testing.T) {
	// Deliberately opt-in: this test executes a reviewed, root-owned worker
	// and should be run only in a trusted privileged test environment,
	// not by enabling sudo for arbitrary untrusted PR test code.
	if os.Geteuid() != 0 {
		t.Skip("requires privileged supervisor in a trusted environment")
	}
	executable := os.Getenv("AGENT_COMPONENT_TEST_ROOT_OWNED_WORKER")
	if executable == "" {
		t.Skip("set AGENT_COMPONENT_TEST_ROOT_OWNED_WORKER to a root-owned installed worker binary")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := RunEngineSharedAsUID(ctx, executable, 65534, 65534, 8, 512, func(session *componentipc.Session, ring *componentshm.Ring) error {
		if session.PeerRole() != componentipc.RoleEngine {
			return componentipc.ErrRoleDenied
		}
		for i := 0; i < 4; i++ {
			ev := componentwire.Event{
				SchemaVersion: componentwire.EnvelopeVersion,
				EventID:       "isolated-" + string(rune('a'+i)),
				Source:        "kernel",
				TimestampNS:   uint64(i+1),
				Type:          "exec",
			}
			if err := SendSharedBatch(session, ring, []componentwire.Event{ev}); err != nil {
				return err
			}
		}
		if stats := ring.Stats(); stats.Published != 4 || stats.Consumed != 4 || stats.Queued != 0 {
			t.Fatalf("privileged root Collector did not observe authenticated child ACKs: %+v", stats)
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("worker file or executable path is not accessible to dropped UID: %v", err)
		}
		t.Fatalf("root-to-UID-65534 Engine supervision: %v", err)
	}
}
