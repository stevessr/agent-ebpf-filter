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


func TestWinParseIPv4UDPBinding(t *testing.T) {
    buf := make([]byte, 4+12)
    binary.LittleEndian.PutUint32(buf[:4],1)
    copy(buf[4:8],[]byte{0,0,0,0})
    copy(buf[8:10],[]byte{0x14,0xE9}) // port 5353
    binary.LittleEndian.PutUint32(buf[12:16],123)
    got, err := winParseUDPRows(buf,winAF_INET)
    if err != nil || len(got)!=1 { t.Fatalf("IPv4 UDP parse: %v / %#v",err,got) }
    for _, b := range got {
        if b.PID != 123 || b.Local != "0.0.0.0:5353" { t.Fatalf("wrong UDP binding: %+v",b) }
    }
}

func TestWinParseIPv6UDPBindingAndScope(t *testing.T) {
    buf := make([]byte,4+28)
    binary.LittleEndian.PutUint32(buf[:4],1)
    // fe80::1%3 local address in network byte order
    buf[4], buf[5], buf[19] = 0xfe,0x80,1
    binary.BigEndian.PutUint32(buf[20:24],3)
    copy(buf[24:26],[]byte{0,53})
    binary.LittleEndian.PutUint32(buf[28:32],345)
    got,err := winParseUDPRows(buf,winAF_INET6)
    if err != nil || len(got)!=1 { t.Fatalf("IPv6 UDP parse: %v / %#v",err,got) }
    for _, b := range got {
        if b.PID != 345 || b.Local != "[fe80::1%3]:53" { t.Fatalf("wrong scoped binding: %+v",b) }
    }
    if _, err := winParseUDPRows(buf[:10],winAF_INET6); err == nil {
        t.Fatal("truncated UDP buffer accepted")
    }
}

func TestWinFetchIPTableRetriesGrowthAndBounds(t *testing.T) {
    attempts := 0
    buf, err := winFetchIPTable("fixture",func(ptr uintptr,size *uint32)uint32{
        attempts++
        if ptr == 0 { *size = 4; return 122 }
        if attempts == 2 { *size = 8; return 122 }
        *size = 8
        return 0
    })
    if err != nil || len(buf)!=8 || attempts!=3 {
        t.Fatalf("retry result: bytes=%d, attempts=%d, err=%v",len(buf),attempts,err)
    }
    _,err = winFetchIPTable("too-large",func(ptr uintptr,size *uint32)uint32{
        *size = winMaxIPTableBytes+1
        return 122
    })
    if err == nil { t.Fatal("accepted unbounded IP Helper allocation") }
}

func TestWinParseIPv6TCPScopedAddress(t *testing.T) {
    buf:=make([]byte,4+56)
    binary.LittleEndian.PutUint32(buf[:4],1)
    row:=buf[4:]
    row[0],row[1],row[15] = 0xfe,0x80,1
    row[24],row[25],row[39] = 0xfe,0x80,2
    binary.BigEndian.PutUint32(row[16:20],4)
    binary.BigEndian.PutUint32(row[40:44],5)
    copy(row[20:22],[]byte{0,100})
    copy(row[44:46],[]byte{0,110})
    binary.LittleEndian.PutUint32(row[48:52],winTCP_STATE_ESTAB)
    binary.LittleEndian.PutUint32(row[52:56],20)
    got,err:=winParseTCPRows(buf,winAF_INET6)
    if err!=nil || len(got)!=1 {t.Fatalf("scoped TCP parse: %v / %#v",err,got)}
    for _,conn:=range got {
        if conn.Local!="[fe80::1%4]:100" || conn.Remote!="[fe80::2%5]:110"{
            t.Fatalf("wrong scope ids: %+v",conn)
        }
    }
}
