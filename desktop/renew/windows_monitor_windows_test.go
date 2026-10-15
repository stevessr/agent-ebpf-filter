//go:build windows

package main

import (
    "encoding/binary"
    "syscall"
    "testing"
)

func TestWindowsTCPRowDecodeV4(t *testing.T) {
    table := make([]byte, 28)
    binary.LittleEndian.PutUint32(table[:4], 1)
    row := table[4:]
    binary.LittleEndian.PutUint32(row[0:4], windowsTCPStateEstablished)
    copy(row[4:8], []byte{127, 0, 0, 1})
    copy(row[8:12], []byte{0x1f, 0x90})
    copy(row[12:16], []byte{1, 2, 3, 4})
    copy(row[16:20], []byte{1, 0xbb})
    binary.LittleEndian.PutUint32(row[20:24], 4321)
    rows, err := decodeWindowsTCPRows(table, syscall.AF_INET)
    if err != nil { t.Fatal(err) }
    if len(rows) != 1 || rows[0].PID != 4321 ||
        rows[0].Local != "127.0.0.1:8080" || rows[0].Remote != "1.2.3.4:443" {
        t.Fatalf("unexpected v4 rows: %+v", rows)
    }
}

func TestWindowsTCPRowRejectsTruncation(t *testing.T) {
    table := make([]byte, 4)
    binary.LittleEndian.PutUint32(table, 1)
    if _, err := decodeWindowsTCPRows(table, syscall.AF_INET6); err == nil {
        t.Fatal("expected bounds check failure")
    }
}

func TestWindowsEBPFMapParse(t *testing.T) {
    payload := []byte(`[
        {"key":["00","00","00","00"],"value":["03","00","00","00","00","00","00","00"]},
        {"key":["01","00","00","00"],"value":["05","00","00","00","00","00","00","00"]}
    ]`)
    a, b, ok := parseWindowsEBPFMap(payload)
    if !ok || a != 3 || b != 5 { t.Fatalf("got %d, %d, %v", a, b, ok) }
}

func TestWindowsEBPFMapRejectsPartial(t *testing.T) {
    if _, _, ok := parseWindowsEBPFMap([]byte(`[{"key":["00","00","00","00"],"value":["01"]}]`)); ok {
        t.Fatal("partial map must not be treated as zero")
    }
}

func TestWindowsAgentAttributionRejectsGenericShell(t *testing.T) {
    processes := map[int]systemProcess{
        12: {PID: 12, Name: "powershell.exe", PPID: 0},
        13: {PID: 13, Name: "node.exe", PPID: 12},
    }
    if pid, _ := windowsAgentForPID(13, processes); pid != 0 {
        t.Fatal("generic node process must not imply agent ownership")
    }
}
