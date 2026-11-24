package tls

import (
	"debug/elf"
	"encoding/binary"
	"errors"
)

type fdeRange struct{ start, end uint64 }

// parseEHFrameFDEs extracts all FDE pc ranges from .eh_frame. It parses the
// CIE/FDE structure directly rather than shelling out to objdump, so it works
// without external tooling at runtime.
func parseEHFrameFDEs(binPath string) ([]fdeRange, error) {
	exe, err := elf.Open(binPath)
	if err != nil {
		return nil, err
	}
	defer exe.Close()

	sec := exe.Section(".eh_frame")
	if sec == nil {
		return nil, errors.New(".eh_frame section not found")
	}
	data, err := sec.Data()
	if err != nil {
		return nil, err
	}
	ehVA := sec.Addr

	var out []fdeRange
	// .eh_frame is a sequence of length-prefixed CIE/FDE entries.
	// We only need FDEs (which carry the pc_begin/pc_range). Each entry begins
	// with a 4-byte length (or 12-byte extended length if length==0xffffffff).
	// The first 4 bytes after length are the CIE pointer; for an FDE this is a
	// non-zero 32-bit offset (relative to the current entry), for a CIE it is 0.
	is64bit := exe.Class == elf.ELFCLASS64
	var endian binary.ByteOrder = binary.LittleEndian
	if exe.Data == elf.ELFDATA2MSB {
		endian = binary.BigEndian
	}

	pos := 0
	for pos+4 <= len(data) {
		length := endian.Uint32(data[pos:])
		entryStart := pos + 4
		var entryLen int
		if length == 0 {
			break // terminator
		}
		if length == 0xffffffff {
			// extended length (64-bit)
			if entryStart+8 > len(data) {
				break
			}
			entryLen = int(endian.Uint64(data[entryStart:]))
			entryStart += 8
		} else {
			entryLen = int(length)
		}
		entryEnd := entryStart + entryLen
		if entryEnd > len(data) {
			break
		}
		body := data[entryStart:entryEnd]

		// CIE pointer (4 bytes for 32-bit DWARF). If zero -> this is a CIE.
		if len(body) < 4 {
			pos = entryEnd
			continue
		}
		ciePtr := endian.Uint32(body)
		if ciePtr == 0 {
			// CIE — skip (but we rely on its address_size; assume 8 for x86_64).
			pos = entryEnd
			continue
		}
		// FDE. Parse PC begin and range using the CIE's augmentation encoding.
		// We need the CIE to know the FDE encoding (often DW_EH_PE_pcrel|sdata4
		// for pc_begin and DW_EH_PE_udata4 for range, the GCC/Linux default).
		// fdeVA points at the start of this FDE's length word.
		fdeVA := ehVA + uint64(pos)
		cieOff := uint64(ciePtr) // offset from the FDE's CIE pointer field back to its CIE
		_ = cieOff
		// Locate CIE body to read augmentation string and FDE encoding.
		cieBody, fdeEnc, ok := readCIEForFDE(data, ehVA, fdeVA, uint64(ciePtr), endian, is64bit)
		if !ok {
			pos = entryEnd
			continue
		}
		_ = cieBody
		// FDE body layout after the CIE pointer:
		//   pc_begin (per fdeEnc), pc_range (per rangeEnc from CIE aug 'R'),
		//   aug data, then CFI instructions.
		// Default GCC encoding: pc_begin = DW_EH_PE_pcrel|sdata4, range = udata4.
		// fdeBodyVA is the vaddr of the pc_begin field itself (the pcrel base).
		fdeBodyVA := fdeVA + 8
		pcBegin, pcRange, consumed, ok := parseFDEAddrs(body[4:], fdeBodyVA, fdeEnc, endian)
		if !ok {
			pos = entryEnd
			continue
		}
		_ = consumed
		if pcRange > 0 {
			out = append(out, fdeRange{start: pcBegin, end: pcBegin + pcRange})
		}
		pos = entryEnd
	}
	return out, nil
}

// readCIEForFDE locates and parses the CIE associated with an FDE to extract
// the FDE address-encoding byte (augmentation 'R'). Returns the CIE body,
// the FDE encoding, and ok.
func readCIEForFDE(data []byte, ehVA, fdeVA, ciePtr uint64, endian binary.ByteOrder, is64bit bool) ([]byte, byte, bool) {
	// ciePtr is the byte offset from the location of the CIE pointer field
	// itself back to the associated CIE (start of its length field).
	ciePtrFieldVA := fdeVA + 4 // CIE pointer field sits right after the FDE length word
	cieLenFieldVA := ciePtrFieldVA - ciePtr
	cieLenFieldOff := int(cieLenFieldVA - ehVA)
	if cieLenFieldOff < 0 || cieLenFieldOff+4 > len(data) {
		return nil, 0, false
	}
	cieLen := endian.Uint32(data[cieLenFieldOff:])
	cieBodyStart := cieLenFieldOff + 4
	var cieBodyLen int
	if cieLen == 0xffffffff {
		if cieBodyStart+8 > len(data) {
			return nil, 0, false
		}
		cieBodyLen = int(endian.Uint64(data[cieBodyStart:]))
		cieBodyStart += 8
	} else {
		cieBodyLen = int(cieLen)
	}
	if cieBodyStart+cieBodyLen > len(data) || cieBodyLen < 4 {
		return nil, 0, false
	}
	cie := data[cieBodyStart : cieBodyStart+cieBodyLen]
	// CIE body: version (1 byte), aug string (null-terminated), then
	// (for v1+) code_alignment_factor (uleb), data_alignment_factor (sleb),
	// return_address_register (uleb for v3, 1 byte for v1), then aug data.
	p := 0
	if len(cie) < 5 {
		return nil, 0, false
	}
	version := cie[p]
	p++
	// augmentation string
	augStart := p
	for p < len(cie) && cie[p] != 0 {
		p++
	}
	aug := string(cie[augStart:p])
	p++ // skip null
	if version >= 4 {
		p += 2 // address_size, segment_size
	}
	// uleb: code_align
	_, n := readULEB(cie[p:])
	p += n
	// sleb: data_align
	_, n = readSLEB(cie[p:])
	p += n
	// return addr reg
	if version >= 3 {
		_, n = readULEB(cie[p:])
		p += n
	} else {
		p++ // 1 byte
	}
	// augmentation data: only if aug starts with 'z'
	fdeEnc := byte(0x1B) // default: pcrel | sdata4 (the common Linux/GCC default)
	if len(aug) > 0 && aug[0] == 'z' {
		augLen, n := readULEB(cie[p:])
		p += n
		augData := cie[p : p+int(augLen)]
		ap := 0
		for i := 1; i < len(aug); i++ {
			switch aug[i] {
			case 'R':
				if ap < len(augData) {
					fdeEnc = augData[ap]
					ap++
				}
			case 'L', 'B', 'P':
				ap++ // one byte each
			case 'S':
				// no data
			}
		}
	}
	return cie, fdeEnc, true
}

// parseFDEAddrs decodes pc_begin (per enc) and pc_range (udata4 default) from
// the FDE body after the CIE pointer. fdeBodyVA is the vaddr of the first byte
// after the CIE pointer field (used for pcrel resolution).
func parseFDEAddrs(body []byte, fdeBodyVA uint64, enc byte, endian binary.ByteOrder) (pcBegin, pcRange uint64, consumed int, ok bool) {
	// Decode pc_begin according to enc. We handle the common formats used by
	// GCC/clang on Linux x86_64/aarch64: pcrel|sdata4 (0x1B), absptr (0x00),
	// textrel|sdata4, udata4.
	pcBegin, n, ok := decodeEHHdr(body, fdeBodyVA, enc, endian)
	if !ok {
		return 0, 0, 0, false
	}
	body = body[n:]
	// pc_range is always an unsigned value in the same size class as the
	// pc_begin encoding's size (per .eh_frame spec). Use the size implied by enc.
	size := ehEncSize(enc)
	if size == 0 {
		size = 4
	}
	if len(body) < size {
		return 0, 0, 0, false
	}
	switch size {
	case 4:
		pcRange = uint64(endian.Uint32(body))
	case 8:
		pcRange = endian.Uint64(body)
	case 2:
		pcRange = uint64(endian.Uint16(body))
	default:
		return 0, 0, 0, false
	}
	return pcBegin, pcRange, n + size, true
}

// decodeEHHdr decodes one DW_EH_PE value from data at vaddr fieldVA.
func decodeEHHdr(data []byte, fieldVA uint64, enc byte, endian binary.ByteOrder) (val uint64, n int, ok bool) {
	if enc == 0xFF { // DW_EH_PE_omit
		return 0, 0, false
	}
	size := ehEncSize(enc)
	if size == 0 || len(data) < size {
		return 0, 0, false
	}
	var raw uint64
	switch size {
	case 4:
		raw = uint64(endian.Uint32(data))
	case 8:
		raw = endian.Uint64(data)
	case 2:
		raw = uint64(endian.Uint16(data))
	default:
		return 0, 0, false
	}
	n = size
	// Sign-extend signed data formats BEFORE applying the base, so pcrel math is correct.
	fmt2 := enc & 0x0F
	switch fmt2 {
	case 0x0A: // sdata2
		if raw&0x8000 != 0 {
			raw |= 0xFFFFFFFFFFFF0000
		}
	case 0x0B: // sdata4
		if raw&0x80000000 != 0 {
			raw |= 0xFFFFFFFF00000000
		}
	}
	// Apply modifier (high 4 bits = base).
	mod := enc >> 4
	switch mod {
	case 0x0: // absptr
		val = raw
	case 0x1: // pcrel
		// pcrel is relative to the address of the field itself.
		val = fieldVA + raw
	case 0x2: // textrel
		val = raw // approximate (text base unknown here); rare
	case 0x3: // datarel
		val = raw
	default:
		val = raw
	}
	return val, n, true
}

func ehEncSize(enc byte) int {
	// DW_EH_PE format is in the low nibble; size class in bytes (0 = variable/omit).
	//   0x00 absptr  -> pointer size (8 on 64-bit)
	//   0x01 uleb128 -> variable (caller must handle; we treat as unknown here)
	//   0x02 udata2  -> 2
	//   0x03 udata4  -> 4
	//   0x04 udata8  -> 8
	//   0x09 sleb128 -> variable
	//   0x0A sdata2  -> 2
	//   0x0B sdata4  -> 4
	//   0x0C sdata8  -> 8
	switch enc & 0x0F {
	case 0x00: // absptr — pointer size
		return 8
	case 0x02: // udata2
		return 2
	case 0x03: // udata4
		return 4
	case 0x04: // udata8
		return 8
	case 0x0A: // sdata2
		return 2
	case 0x0B: // sdata4
		return 4
	case 0x0C: // sdata8
		return 8
	}
	return 0
}

func readULEB(b []byte) (uint64, int) {
	var val uint64
	var shift uint
	for i := 0; i < len(b); i++ {
		c := b[i]
		val |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return val, i + 1
		}
		shift += 7
	}
	return val, len(b)
}

func readSLEB(b []byte) (int64, int) {
	var val int64
	var shift uint
	for i := 0; i < len(b); i++ {
		c := b[i]
		val |= int64(c&0x7f) << shift
		shift += 7
		if c&0x80 == 0 {
			if c&0x40 != 0 {
				val |= -1 << shift
			}
			return val, i + 1
		}
	}
	return val, len(b)
}
