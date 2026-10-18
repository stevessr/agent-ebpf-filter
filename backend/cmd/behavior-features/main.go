// behavior-features is an offline JSONL cleaner. It does not attach probes,
// execute payloads, contact destinations, or enable policy enforcement.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"agent-ebpf-filter/internal/behaviorfeatures"
	"agent-ebpf-filter/internal/strictjson"
)

func run(in io.Reader, out io.Writer, batch bool, catalog bool) error {
	enc := json.NewEncoder(out)
	if catalog {
		return enc.Encode(behaviorfeatures.Catalog())
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), 512*1024)
	var records []behaviorfeatures.Input
	bytesRead := 0
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var i behaviorfeatures.Input
		if err := strictjson.Decode(scanner.Bytes(), &i); err != nil {
			return err
		}
		if batch {
			bytesRead += len(scanner.Bytes())
			if bytesRead > behaviorfeatures.MaxBatchBytes {
				return fmt.Errorf("batch exceeds byte limit")
			}
			if len(records) >= behaviorfeatures.MaxEvents {
				return fmt.Errorf("batch exceeds event limit")
			}
			records = append(records, i)
		} else {
			o, e := behaviorfeatures.Extract(i)
			if e != nil {
				return e
			}
			if e = enc.Encode(o); e != nil {
				return e
			}
		}
	}
	if scanner.Err() != nil {
		return fmt.Errorf("input read failed or record exceeds size limit")
	}
	if batch {
		r, e := behaviorfeatures.ExtractBatch(records)
		if e != nil {
			return e
		}
		return enc.Encode(r)
	}
	return nil
}

func main() {
	batch := flag.Bool("batch", false, "bounded JSONL batch with prior-only history")
	catalog := flag.Bool("catalog", false, "print versioned ordered feature names")
	flag.Parse()
	if err := run(os.Stdin, os.Stdout, *batch, *catalog); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
