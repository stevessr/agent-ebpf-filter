package main

import "time"

const tlsFragmentSize = 960
const tlsMaxFragments = 18

const tlsLibOpenSSL = 0
const tlsLibGo = 1
const tlsLibGnuTLS = 2
const tlsLibNSS = 3

const tlsDirectionRecv = 0
const tlsDirectionSend = 1

type tlsFragment struct {
	TimestampNS uint64
	PID         uint32
	TGID        uint32
	DataLen     uint32
	TotalLen    uint32
	FragIndex   uint16
	FragCount   uint16
	LibType     uint8
	Direction   uint8
	Comm        [16]byte
	Data        [tlsFragmentSize]byte
}

type completedTLSFragment struct {
	TimestampNS uint64
	PID         uint32
	TGID        uint32
	DataLen     uint32
	TotalLen    uint32
	FragCount   uint16
	LibType     uint8
	Direction   uint8
	Comm        string
	Payload     []byte
}

type TLSPlaintextEvent struct {
	Type         string            `json:"type"`
	Timestamp    time.Time         `json:"timestamp"`
	PID          uint32            `json:"pid"`
	TGID         uint32            `json:"tgid"`
	Comm         string            `json:"comm"`
	Direction    string            `json:"direction"`
	Lib          string            `json:"lib"`
	Method       string            `json:"method,omitempty"`
	URL          string            `json:"url,omitempty"`
	Host         string            `json:"host,omitempty"`
	StatusCode   int               `json:"status,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	BodySize     int               `json:"body_size"`
	ContentType  string            `json:"content_type,omitempty"`
	RawHexDump   string            `json:"raw_hex_dump,omitempty"`
	RawAvailable bool              `json:"raw_available"`
	Truncated    bool              `json:"truncated"`
}

type TLSLibraryStatus struct {
	Library   uint8  `json:"library"`
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Attached  bool   `json:"attached"`
	Available bool   `json:"available"`
	Error     string `json:"error,omitempty"`
}

type TLSCaptureStats struct {
	Pending      int           `json:"pending"`
	Dropped      int           `json:"dropped"`
	Timeout      time.Duration `json:"timeout"`
	Libraries    []TLSLibraryStatus `json:"libraries,omitempty"`
	LastFragmentNS uint64      `json:"lastFragmentNs,omitempty"`
}
