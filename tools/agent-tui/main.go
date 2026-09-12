// Command agent-tui is a terminal dashboard for a running Agent eBPF Filter
// backend. It subscribes to the same protobuf event feed the web UI uses and
// renders a live, filterable event table with throughput and risk summaries.
//
//	make tui                       # connect to the local dev backend
//	agent-tui -backend http://host:8080 -token "$TOKEN"
//
// The backend origin and token are discovered from flags, then the
// AGENT_BACKEND_URL / AGENT_BACKEND_PORT / AGENT_ACCESS_TOKEN environment, then
// the dev backend's .port file and ~/.config/agent-ebpf-filter/runtime.json.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "agent-tui:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("agent-tui", flag.ContinueOnError)
	backend := flags.String("backend", "", "backend origin, e.g. http://127.0.0.1:8080 (default: auto-detect)")
	token := flags.String("token", "", "API token sent as X-API-KEY (default: env/runtime.json)")
	history := flags.Int("history", defaultHistory, "events kept in memory")
	backfill := flags.Int("backfill", defaultBackfill, "recent events to load before streaming (0 disables)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	backendURL, err := resolveBackendURL(*backend, os.Getenv, cwd)
	if err != nil {
		return err
	}
	cfg := Config{
		BackendURL: backendURL,
		Token:      resolveToken(*token, os.Getenv, realHomeDir()),
		History:    *history,
		Backfill:   *backfill,
	}
	if err := cfg.validate(); err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	model := NewModel(cfg.History)
	tlsModel := NewTLSModel(cfg.History)
	go NewStream(cfg, model).Run(ctx)
	go NewTLSStream(cfg, tlsModel).Run(ctx)

	ui := NewUI(cfg, model, tlsModel)
	if err := ui.Run(ctx.Done()); err != nil {
		return fmt.Errorf("terminal ui: %w", err)
	}
	return nil
}
