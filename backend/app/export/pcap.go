package export

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

// ── Minimal PCAP writer (libpcap format) ──────────────────────────────

const (
	pcapMagicNumber      = 0xa1b2c3d4
	pcapVersionMajor     = 2
	pcapVersionMinor     = 4
	pcapThisZone         = 0
	pcapSigFigs          = 0
	pcapSnapLen          = 65535
	pcapLinkTypeEthernet = 1
)

type pcapGlobalHeader struct {
	MagicNumber  uint32
	VersionMajor uint16
	VersionMinor uint16
	ThisZone     int32
	SigFigs      uint32
	SnapLen      uint32
	LinkType     uint32
}

type pcapPacketHeader struct {
	TSSeconds   uint32
	TSUseconds  uint32
	IncludedLen uint32
	OrigLen     uint32
}

// WritePCAPHeader writes a libpcap file header.
func WritePCAPHeader(f *os.File) error {
	hdr := pcapGlobalHeader{
		MagicNumber:  pcapMagicNumber,
		VersionMajor: pcapVersionMajor,
		VersionMinor: pcapVersionMinor,
		SnapLen:      pcapSnapLen,
		LinkType:     pcapLinkTypeEthernet,
	}
	return binary.Write(f, binary.LittleEndian, hdr)
}

// WritePCAPPacket writes a single packet record to a PCAP file.
func WritePCAPPacket(f *os.File, timestamp time.Time, data []byte) error {
	ts := timestamp.UTC()
	pktHdr := pcapPacketHeader{
		TSSeconds:   uint32(ts.Unix()),
		TSUseconds:  uint32(ts.Nanosecond() / 1000),
		IncludedLen: uint32(len(data)),
		OrigLen:     uint32(len(data)),
	}
	if err := binary.Write(f, binary.LittleEndian, pktHdr); err != nil {
		return err
	}
	_, err := f.Write(data)
	return err
}

// BuildSyntheticEthernetFrame builds a minimal Ethernet frame for PCAP export.
func BuildSyntheticEthernetFrame(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, bytes uint64) []byte {
	frame := make([]byte, 14+20+20)

	frame[12] = 0x08
	frame[13] = 0x00

	frame[14] = 0x45
	frame[15] = 0x00
	binary.BigEndian.PutUint16(frame[16:18], uint16(len(frame)-14))
	frame[18] = 0x00
	frame[19] = 0x01
	frame[20] = 0x00
	frame[21] = 0x00
	frame[22] = 0x40
	frame[23] = 0x06
	frame[29] = 0x06

	ParseIPToBytes(srcIP, frame[26:30])
	ParseIPToBytes(dstIP, frame[30:34])

	binary.BigEndian.PutUint16(frame[34:36], uint16(srcPort))
	binary.BigEndian.PutUint16(frame[36:38], uint16(dstPort))
	frame[38] = 0x00
	frame[39] = 0x00
	frame[40] = 0x00
	frame[41] = 0x01
	frame[46] = 0x50
	frame[47] = 0x02
	frame[48] = 0xFF
	frame[49] = 0xFF

	return frame
}

// ParseIPToBytes parses a dotted-decimal IPv4 string into a byte slice.
func ParseIPToBytes(ipStr string, dst []byte) {
	var a, b, c, d int
	fmt.Sscanf(ipStr, "%d.%d.%d.%d", &a, &b, &c, &d)
	dst[0] = byte(a)
	dst[1] = byte(b)
	dst[2] = byte(c)
	dst[3] = byte(d)
}
