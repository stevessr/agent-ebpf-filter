package tls

import (
	"agent-ebpf-filter/tls"
)

// Global key remover instance
var globalKeyRemover *tls.KeyRemover

// InitTLSKeyRemover initializes the global key remover.
func InitTLSKeyRemover() {
	if globalKeyRemover == nil {
		globalKeyRemover = tls.NewKeyRemover()
	}
}

// GetTLSKeyRemover returns the global key remover instance.
func GetTLSKeyRemover() *tls.KeyRemover {
	if globalKeyRemover == nil {
		InitTLSKeyRemover()
	}
	return globalKeyRemover
}

// RemoveSensitiveDataFromTLS removes sensitive data from TLS plaintext.
// This should be called before storing or broadcasting TLS events.
func RemoveSensitiveDataFromTLS(data []byte) []byte {
	kr := GetTLSKeyRemover()
	return kr.RemoveSensitiveData(data)
}

// RemoveSensitiveStringFromTLS is a convenience wrapper for string data.
func RemoveSensitiveStringFromTLS(data string) string {
	kr := GetTLSKeyRemover()
	return kr.RemoveSensitiveString(data)
}
