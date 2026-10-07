package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWrapperRewriteCommandAcceptsArrayAndLegacyString(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "argv", raw: `["echo","hello world"]`, want: []string{"echo", "hello world"}},
		{name: "legacy string", raw: `"echo hello"`, want: []string{"echo hello"}},
		{name: "empty legacy string", raw: `""`, want: nil},
		{name: "null", raw: `null`, want: nil},
		{name: "invalid object", raw: `{"cmd":"echo"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got wrapperRewriteCommand
			err := json.Unmarshal([]byte(tt.raw), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected decode error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual([]string(got), tt.want) {
				t.Fatalf("decoded = %#v, want %#v", []string(got), tt.want)
			}
		})
	}
}
