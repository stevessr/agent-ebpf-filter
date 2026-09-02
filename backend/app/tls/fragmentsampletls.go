package tls

import (
	"encoding/binary"
	"fmt"
)

// tlsFragmentMetadataSize is the byte offset of tls_fragment.data in the BPF C
// struct. The metadata fields are naturally packed to 60 bytes; the C struct's
// trailing alignment padding is deliberately not part of the wire contract.
const tlsFragmentMetadataSize = 60

// decodeTLSFragmentSample accepts both the legacy fixed-size perf sample
// (sizeof(struct tls_fragment), currently 1024 bytes) and the compact wire
// format (metadata header + DataLen bytes). Keeping the decoder dual-format
// makes the BPF/userspace transition backwards compatible and simplifies
// rollback across mixed binaries.
func decodeTLSFragmentSample(raw []byte) (tlsFragment, error) {
	var fragment tlsFragment
	if len(raw) < tlsFragmentMetadataSize {
		return fragment, fmt.Errorf("TLS perf sample too short: got %d want >= %d", len(raw), tlsFragmentMetadataSize)
	}

	fragment.TimestampNS = binary.LittleEndian.Uint64(raw[0:8])
	fragment.ConnectionID = binary.LittleEndian.Uint64(raw[8:16])
	fragment.PID = binary.LittleEndian.Uint32(raw[16:20])
	fragment.TGID = binary.LittleEndian.Uint32(raw[20:24])
	fragment.DataLen = binary.LittleEndian.Uint32(raw[24:28])
	fragment.TotalLen = binary.LittleEndian.Uint32(raw[28:32])
	fragment.OriginalLen = binary.LittleEndian.Uint32(raw[32:36])
	fragment.FragIndex = binary.LittleEndian.Uint16(raw[36:38])
	fragment.FragCount = binary.LittleEndian.Uint16(raw[38:40])
	fragment.LibType = raw[40]
	fragment.Direction = raw[41]
	fragment.Flags = raw[42]
	fragment.Function = raw[43]
	copy(fragment.Comm[:], raw[44:60])

	if fragment.DataLen == 0 || fragment.DataLen > tlsFragmentSize {
		return tlsFragment{}, fmt.Errorf("invalid TLS perf sample data_len=%d", fragment.DataLen)
	}
	wireLen := tlsFragmentMetadataSize + int(fragment.DataLen)
	if len(raw) < wireLen {
		return tlsFragment{}, fmt.Errorf("truncated TLS perf sample: got %d want >= %d", len(raw), wireLen)
	}
	copy(fragment.Data[:fragment.DataLen], raw[tlsFragmentMetadataSize:wireLen])
	return fragment, nil
}
