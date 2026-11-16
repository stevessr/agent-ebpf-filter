// behavior-risk is a bounded stdin/stdout shadow worker, outside the eBPF hot
// path. It never opens a network listener or emits enforcement decisions.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"agent-ebpf-filter/internal/behaviorfeatures"
	"agent-ebpf-filter/internal/behaviorrisk"
	"agent-ebpf-filter/internal/strictjson"
)

type counts struct {
	Records   int `json:"records"`
	Scored    int `json:"scored"`
	Abstained int `json:"abstained"`
	Invalid   int `json:"invalid"`
}

func run(in io.Reader, out, diagnostic io.Writer, manager *behaviorrisk.Manager, vectors bool) error {
	s := bufio.NewScanner(in)
	s.Buffer(make([]byte, 4096), 512*1024)
	enc := json.NewEncoder(out)
	c := counts{}
	for s.Scan() {
		c.Records++
		var i behaviorfeatures.Input
		if e := strictjson.Decode(s.Bytes(), &i); e != nil {
			c.Invalid++
			c.Abstained++
			if e = enc.Encode(behaviorrisk.Result{Mode: "shadow_only", Status: "abstain"}); e != nil {
				return e
			}
			continue
		}
		if vectors {
			text, kind, e := behaviorrisk.Text(i)
			if e != nil {
				c.Abstained++
				if e = enc.Encode(behaviorrisk.Result{Mode: "shadow_only", Status: "abstain"}); e != nil {
					return e
				}
				continue
			}
			v, e := behaviorrisk.Encode(text, kind)
			if e != nil {
				return e
			}
			if e = enc.Encode(v); e != nil {
				return e
			}
			c.Scored++
		} else {
			r := manager.Score(i)
			if r.Score == nil {
				c.Abstained++
			} else {
				c.Scored++
			}
			if e := enc.Encode(r); e != nil {
				return e
			}
		}
	}
	if e := json.NewEncoder(diagnostic).Encode(c); e != nil {
		return e
	}
	if s.Err() != nil {
		return fmt.Errorf("input read failed or record limit exceeded")
	}
	return nil
}
func main() {
	vectors := flag.Bool("vectors", false, "offline feature export; may encode sensitive text, use only approved training data")
	model := flag.String("model", "", "shadow model JSON")
	sha := flag.String("sha256", "", "expected model checksum from trusted manifest")
	socket := flag.String("socket", "", "optional private Unix socket; parent must be owned 0700 directory")
	flag.Parse()
	if *vectors && *socket != "" {
		fmt.Fprintln(os.Stderr, "feature export cannot run as a socket service")
		os.Exit(1)
	}
	manager := &behaviorrisk.Manager{}
	if !*vectors {
		f, e := os.Open(*model)
		if e != nil {
			fmt.Fprintln(os.Stderr, "model open failed")
			os.Exit(1)
		}
		e = manager.Reload(f, *sha)
		f.Close()
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
	}
	if *socket != "" {
		listener, e := behaviorrisk.ListenPrivate(*socket)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		server := &behaviorrisk.Server{Manager: manager}
		e = server.Serve(ctx, listener)
		json.NewEncoder(os.Stderr).Encode(server.Stats())
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	if e := run(os.Stdin, os.Stdout, os.Stderr, manager, *vectors); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
