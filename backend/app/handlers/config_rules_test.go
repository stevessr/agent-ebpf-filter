package handlers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRewrittenCommandAcceptsArrayAndLegacyString(t *testing.T) {
	var array rewrittenCommand
	if err := json.Unmarshal([]byte(`["echo","hello"]`), &array); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string(array), []string{"echo", "hello"}) {
		t.Fatalf("array decode = %#v", array)
	}

	var legacy rewrittenCommand
	if err := json.Unmarshal([]byte(`"echo"`), &legacy); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string(legacy), []string{"echo"}) {
		t.Fatalf("legacy decode = %#v", legacy)
	}
}
