// Package strictjson rejects ambiguous JSON before decoding untrusted records.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Decode requires exact JSON field spellings, unique keys and bounded depth.
// Errors contain no input values. Size limits belong to the caller's reader.
func Decode(data []byte, dst any) error {
	fail := errors.New("invalid or ambiguous JSON record")
	t := reflect.TypeOf(dst)
	if t == nil || t.Kind() != reflect.Pointer || !utf8.Valid(data) {
		return fail
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if check(d, t.Elem(), 0) != nil {
		return fail
	}
	if _, e := d.Token(); e != io.EOF {
		return fail
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(dst) != nil {
		return fail
	}
	return nil
}
func check(d *json.Decoder, t reflect.Type, depth int) error {
	if depth > 32 {
		return errors.New("depth")
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	tok, e := d.Token()
	if e != nil {
		return e
	}
	if tok == nil {
		return nil
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		if t.Kind() != reflect.Struct {
			return errors.New("object type")
		}
		fields := map[string]reflect.Type{}
		for n := 0; n < t.NumField(); n++ {
			f := t.Field(n)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				fields[name] = f.Type
			}
		}
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return e
			}
			name, ok := k.(string)
			ft, known := fields[name]
			if !ok || !known || seen[name] {
				return errors.New("key")
			}
			seen[name] = true
			if e = check(d, ft, depth+1); e != nil {
				return e
			}
		}
	case '[':
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return errors.New("array type")
		}
		for d.More() {
			if e = check(d, t.Elem(), depth+1); e != nil {
				return e
			}
		}
	default:
		return errors.New("delimiter")
	}
	_, e = d.Token()
	return e
}
