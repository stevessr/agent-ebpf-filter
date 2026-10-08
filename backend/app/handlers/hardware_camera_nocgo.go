//go:build !cgo

package handlers

func listCameraDevices() []string {
	return []string{}
}
