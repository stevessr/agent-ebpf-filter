//go:build android && cgo

package main

import (
	"log"

	"github.com/egoist/mygo/android/jni"
)

// Android's Activity owns the process. JNI calls this callback once the
// native Surface is ready; a c-shared Go library never invokes main itself.
type androidLogWriter struct{}
func (androidLogWriter) Write(p []byte) (int, error) {
	jni.Log(string(p))
	return len(p), nil
}

func init() {
	log.SetOutput(androidLogWriter{})
	jni.Start(main)
}
