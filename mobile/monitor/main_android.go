//go:build android && cgo

package main

// MyGo v0.10.0+ includes the Android JNI bridge and logcat stdout/stderr
// forwarding in internal/mobile (selected automatically via backend_mobile.go
// when building with GOOS=android CGO_ENABLED=1 -buildmode=c-shared).
