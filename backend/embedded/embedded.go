// Package embedded exposes the privileged Agent eBPF Filter runtime as an
// in-process Go library entry point. Desktop builds link this package into the
// Renew executable and re-exec the same binary in backend mode after privilege
// elevation, so no sidecar backend executable is required.
package embedded

import "agent-ebpf-filter/app"

// Run configures desktop-specific backend flags and starts the backend runtime.
// args must not include the Renew-only --internal-backend dispatch flag.
func Run(args []string) error {
	if err := app.ConfigureDesktopFlags(args); err != nil {
		return err
	}
	return app.Main()
}
