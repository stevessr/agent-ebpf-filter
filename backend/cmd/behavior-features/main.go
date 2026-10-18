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
		if err := uniqueJSON(json.NewDecoder(bytes.NewReader(scanner.Bytes())), 0); err != nil {
			return fmt.Errorf("duplicate keys or invalid JSON")
		}
		dec := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&i); err != nil {
			return fmt.Errorf("invalid canonical input at record boundary")
		}
		var extra any
		if dec.Decode(&extra) != io.EOF {
			return fmt.Errorf("trailing JSON input")
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

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("JSON depth limit")
	}
	tok, e := d.Token()
	if e != nil {
		return e
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return fmt.Errorf("duplicate object key")
				}
				seen[s] = true
				if e = uniqueJSON(d, depth+1); e != nil {
					return e
				}
			}
		case '[':
			for d.More() {
				if e = uniqueJSON(d, depth+1); e != nil {
					return e
				}
			}
		default:
			return fmt.Errorf("invalid delimiter")
		}
		_, e = d.Token()
		return e
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
