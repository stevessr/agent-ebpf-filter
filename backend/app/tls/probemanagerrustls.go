package tls

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf/link"
)

// AttachRustlsUprobes attaches the rustls-specific uprobes inside a stripped
// static-pie Rust binary (codex, cursor). The offsets are discovered via
// .eh_frame + .rodata anchor cross-referencing in probediscoveryrustls.go.
func (m *TLSProbeManager) AttachRustlsUprobes(binPath string, pid int) error {
	if m == nil {
		return nil
	}

	offsets, err := FindRustlsOffsets(binPath)
	if err != nil {
		return fmt.Errorf("find rustls offsets: %w", err)
	}
	if offsets.WriteTLS == 0 && offsets.ReadTLS == 0 {
		return fmt.Errorf("no rustls functions found")
	}

	bin, err := link.OpenExecutable(binPath)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.objs == nil {
		return fmt.Errorf("TLS probe manager is closed")
	}

	opts := &link.UprobeOptions{}
	if pid > 0 {
		opts.PID = pid
	}

	startLinks := len(m.links)
	var errs []error

	if offsets.WriteTLS > 0 {
		opts.Address = offsets.WriteTLS
		if prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, "uprobe_rustls_encrypt_outgoing"); ok && prog != nil {
			l, err := bin.Uprobe("", prog, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("uprobe rustls_encrypt_outgoing@0x%x: %w", offsets.WriteTLS, err))
			} else {
				m.links = append(m.links, l)
			}
		} else {
			errs = append(errs, fmt.Errorf("uprobe_rustls_encrypt_outgoing program not loaded"))
		}
	}

	if offsets.ReadTLS > 0 {
		opts.Address = offsets.ReadTLS
		if prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, "uprobe_rustls_consume_first_chunk"); ok && prog != nil {
			l, err := bin.Uprobe("", prog, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("uprobe rustls_consume_first_chunk@0x%x: %w", offsets.ReadTLS, err))
			} else {
				m.links = append(m.links, l)
			}
		} else {
			errs = append(errs, fmt.Errorf("uprobe_rustls_consume_first_chunk program not loaded"))
		}
	}

	if err := errors.Join(errs...); err != nil {
		for _, l := range m.links[startLinks:] {
			if l != nil {
				_ = l.Close()
			}
		}
		m.links = m.links[:startLinks]
		return err
	}

	m.registerPIDLinkRangeLocked(pid, startLinks)
	if m.store != nil {
		m.store.SetLibraryStatus(TLSLibraryStatus{
			Name:      "rustls",
			Path:      binPath,
			Attached:  true,
			Available: true,
		})
	}
	return nil
}
