//go:build windows

package main

import (
    "encoding/binary"
    "fmt"
    "net"
    "net/netip"
    "strconv"
    "syscall"
    "unsafe"
)

const (
    winUDP_TABLE_OWNER_PID = 1
    winMaxIPTableBytes = 16 << 20
    winMaxIPTableRetries = 4
)

var winGetExtendedUdpTable = winIPHLPAPI.NewProc("GetExtendedUdpTable")

// IP Helper's table can grow between the length query and the next read.
// Retry boundedly; do not trust a row count or allocate an unbounded buffer.
func winFetchIPTable(name string, query func(buffer uintptr, size *uint32) uint32) ([]byte, error) {
    var size uint32
    status := query(0, &size)
    if status != 0 && syscall.Errno(status) != winERROR_INSUFFICIENT_BUFFER {
        return nil, fmt.Errorf("%s sizing: %w", name, syscall.Errno(status))
    }
    for attempt := 0; attempt < winMaxIPTableRetries; attempt++ {
        if size < 4 || size > winMaxIPTableBytes {
            return nil, fmt.Errorf("%s invalid table length %d", name, size)
        }
        buffer := make([]byte, size)
        gotSize := size
        status = query(uintptr(unsafe.Pointer(&buffer[0])), &gotSize)
        if syscall.Errno(status) == winERROR_INSUFFICIENT_BUFFER {
            size = gotSize
            continue
        }
        if status != 0 { return nil, fmt.Errorf("%s read: %w", name, syscall.Errno(status)) }
        if gotSize < 4 || gotSize > uint32(len(buffer)) {
            return nil, fmt.Errorf("%s returned inconsistent table length %d", name, gotSize)
        }
        return buffer[:gotSize], nil
    }
    return nil, fmt.Errorf("%s changed while reading after %d attempts", name, winMaxIPTableRetries)
}

func winUDPTable(family uintptr) ([]byte, error) {
    name := fmt.Sprintf("GetExtendedUdpTable(%d)", family)
    return winFetchIPTable(name, func(buffer uintptr, size *uint32) uint32 {
        status, _, _ := winGetExtendedUdpTable.Call(buffer, uintptr(unsafe.Pointer(size)),
            0, family, winUDP_TABLE_OWNER_PID, 0)
        return uint32(status)
    })
}

// Scope IDs and ports are supplied in network byte order. Keep zones
// attached to IPv6 addresses so link-local endpoints are not conflated.
func winIPv6Scoped(ip []byte, scope []byte) string {
    addr, ok := netip.AddrFromSlice(ip)
    if !ok { return "" }
    zone := binary.BigEndian.Uint32(scope)
    if zone != 0 { addr = addr.WithZone(strconv.FormatUint(uint64(zone), 10)) }
    return addr.String()
}

// UDP owner PID rows: IPv4=12 bytes, IPv6=28 bytes. These are local
// bindings only; no remote peer or packet is present in IP Helper's table.
func winParseUDPRows(data []byte, family uintptr) (map[string]windowsUDPSample, error) {
    if len(data) < 4 { return nil, fmt.Errorf("UDP table header truncated") }
    size := 12
    switch family {
    case winAF_INET:
    case winAF_INET6: size = 28
    default: return nil, fmt.Errorf("unsupported UDP family %d", family)
    }
    count := binary.LittleEndian.Uint32(data[:4])
    if uint64(count)*uint64(size) > uint64(len(data)-4) {
        return nil, fmt.Errorf("UDP table row count exceeds buffer")
    }
    bindings := make(map[string]windowsUDPSample)
    for i := uint32(0); i < count; i++ {
        row := data[4+int(i)*size:4+(int(i)+1)*size]
        var host string
        var port uint16
        var pid uint32
        if family == winAF_INET {
            host = net.IP(row[0:4]).String()
            port = binary.BigEndian.Uint16(row[4:6])
            pid = binary.LittleEndian.Uint32(row[8:12])
        } else {
            host = winIPv6Scoped(row[0:16], row[16:20])
            port = binary.BigEndian.Uint16(row[20:22])
            pid = binary.LittleEndian.Uint32(row[24:28])
        }
        if pid == 0 || port == 0 { continue }
        local := net.JoinHostPort(host, strconv.Itoa(int(port)))
        key := fmt.Sprintf("udp|%d|%s", pid, local)
        bindings[key] = windowsUDPSample{PID:int(pid), Local:local}
    }
    return bindings, nil
}

func winReadUDPBindings() (map[string]windowsUDPSample, error) {
    all := make(map[string]windowsUDPSample)
    for _, family := range []uintptr{winAF_INET,winAF_INET6} {
        buf, err := winUDPTable(family)
        if err != nil { return nil, err }
        parsed, err := winParseUDPRows(buf, family)
        if err != nil { return nil, err }
        for key, value := range parsed { all[key] = value }
    }
    return all, nil
}
