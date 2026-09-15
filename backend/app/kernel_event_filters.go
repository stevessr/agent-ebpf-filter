package app

import (
	"unicode/utf8"
	"unsafe"
)

// kernelCommLookupKey returns a transient string view over a canonical Linux
// task comm buffer. The returned string is only safe while raw remains alive and
// unchanged; callers must use it for immediate lookup only and must not retain
// it. Non-canonical padding or invalid UTF-8 falls back to the existing sanitize
// path so filtering semantics remain identical for malformed samples.
func kernelCommLookupKey(raw []byte) (string, bool) {
	end := len(raw)
	for index, value := range raw {
		if value != 0 {
			continue
		}
		end = index
		for _, trailing := range raw[index+1:] {
			if trailing != 0 {
				return "", false
			}
		}
		break
	}

	if !utf8.Valid(raw[:end]) {
		return "", false
	}
	if end == 0 {
		return "", true
	}
	return unsafe.String(unsafe.SliceData(raw), end), true
}

func lookupDisabledComm(comm string) bool {
	return lookupDisabledCommSnapshot(comm)
}

// isRawCommDisabled avoids constructing and sanitizing a Go string for the
// normal kernel representation (valid UTF-8 followed only by NUL padding).
// Pathological buffers deliberately use sanitizeUTF8 to preserve the previous
// embedded-NUL and invalid-UTF8 behavior exactly.
func isRawCommDisabled(raw []byte) bool {
	comm, fast := kernelCommLookupKey(raw)
	if !fast {
		comm = sanitizeUTF8(raw)
	}
	return lookupDisabledComm(comm)
}
