//go:build windows

package main

import (
	"encoding/binary"
	"testing"
)

func TestWinParseIPv4EstablishedOwner(t *testing.T) {
	table := make([]byte, 4+24*2)
	binary.LittleEndian.PutUint32(table[:4], 2)
	row := table[4:28]
	binary.LittleEndian.PutUint32(row[0:4], winTCP_STATE_ESTAB)
	copy(row[4:8], []byte{127,0,0,1})
	copy(row[8:10], []byte{0xD2,0x04})
	copy(row[12:16], []byte{1,2,3,4})
	copy(row[16:18], []byte{0x01,0xBB})
	binary.LittleEndian.PutUint32(row[20:24], 42)
	// Listening-only state must not be represented as outbound traffic.
	row = table[28:52]
	binary.LittleEndian.PutUint32(row[0:4], 2)
	binary.LittleEndian.PutUint32(row[20:24], 99)
	got, err := winParseTCPRows(table, winAF_INET)
	if err != nil || len(got) != 1 {
		t.Fatalf("IPv4 parse: %v %#v", err, got)
	}
	for _, conn := range got {
		if conn.PID != 42 || conn.Remote != "1.2.3.4:443" || conn.Local != "127.0.0.1:53764" {
			t.Fatalf("unexpected TCP peer: %+v", conn)
		}
	}
}

func TestWinParseIPv6EstablishedOwner(t *testing.T) {
	table := make([]byte, 4+56)
	binary.LittleEndian.PutUint32(table[:4], 1)
	row := table[4:]
	row[15] = 1
	row[39] = 1
	copy(row[20:22], []byte{0xC0,0x01})
	copy(row[44:46], []byte{0x01,0xBB})
	binary.LittleEndian.PutUint32(row[48:52], winTCP_STATE_ESTAB)
	binary.LittleEndian.PutUint32(row[52:56], 77)
	got, err := winParseTCPRows(table, winAF_INET6)
	if err != nil || len(got) != 1 {
		t.Fatalf("IPv6 parse: %v %#v", err, got)
	}
	for _, conn := range got {
		if conn.PID != 77 || conn.Remote != "[::1]:443" { t.Fatalf("IPv6: %+v", conn) }
	}
}

func TestWinParseTruncatedTCPTable(t *testing.T) {
	table := make([]byte, 6)
	binary.LittleEndian.PutUint32(table[:4], 1000)
	if _, err := winParseTCPRows(table, winAF_INET); err == nil {
		t.Fatal("truncated IP Helper table must not be read")
	}
}
