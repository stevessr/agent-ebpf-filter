package app

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf/link"
)

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

	// 附加 write_tls (发送方向)
	if offsets.WriteTLS > 0 {
		opts.Address = offsets.WriteTLS
		if prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, "uprobe_ssl_write"); ok && prog != nil {
			l, err := bin.Uprobe("", prog, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("uprobe write_tls@0x%x: %w", offsets.WriteTLS, err))
			} else {
				m.links = append(m.links, l)
			}
		}
	}

	// 附加 read_tls (接收方向)
	if offsets.ReadTLS > 0 {
		opts.Address = offsets.ReadTLS
		if prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, "uprobe_ssl_read"); ok && prog != nil {
			l, err := bin.Uprobe("", prog, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("uprobe read_tls@0x%x: %w", offsets.ReadTLS, err))
			} else {
				m.links = append(m.links, l)
			}
		}
		// uretprobe for read
		if prog, ok := programByName(&m.objs.AgentTlsCapturePrograms, "uretprobe_ssl_read"); ok && prog != nil {
			l, err := bin.Uretprobe("", prog, opts)
			if err != nil {
				errs = append(errs, fmt.Errorf("uretprobe read_tls@0x%x: %w", offsets.ReadTLS, err))
			} else {
				m.links = append(m.links, l)
			}
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
