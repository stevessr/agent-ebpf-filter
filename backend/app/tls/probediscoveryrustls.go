package tls

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
)

// RustlsOffsets 存储 rustls 关键函数的偏移量
type RustlsOffsets struct {
	WriteTLS uint64
	ReadTLS  uint64
}

// FindRustlsOffsets 在 stripped binary 中通过模式匹配定位 rustls 函数
// 目标函数：
// - rustls::connection::Connection::write_tls
// - rustls::connection::Connection::read_tls
func FindRustlsOffsets(binPath string) (*RustlsOffsets, error) {
	exe, err := elf.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("open binary: %w", err)
	}
	defer exe.Close()

	// 尝试从符号表获取（非 stripped 情况）
	if offsets := trySymbolTable(exe); offsets != nil {
		return offsets, nil
	}

	// 对于 stripped binary，通过特征模式匹配
	return scanTextSection(exe)
}

func trySymbolTable(exe *elf.File) *RustlsOffsets {
	symbols, _ := exe.Symbols()
	if len(symbols) == 0 {
		symbols, _ = exe.DynamicSymbols()
	}

	var offsets RustlsOffsets
	var foundWrite, foundRead bool

	for _, sym := range symbols {
		// rustls 函数名模式
		if containsRustlsWrite(sym.Name) && sym.Value > 0 {
			offsets.WriteTLS = sym.Value
			foundWrite = true
		}
		if containsRustlsRead(sym.Name) && sym.Value > 0 {
			offsets.ReadTLS = sym.Value
			foundRead = true
		}
	}

	if foundWrite && foundRead {
		return &offsets
	}
	return nil
}

func containsRustlsWrite(name string) bool {
	patterns := []string{
		"rustls::connection::Connection::write_tls",
		"rustls::conn::Connection::write_tls",
		"_ZN6rustls10connection10Connection9write_tls",
	}
	for _, p := range patterns {
		if name == p {
			return true
		}
	}
	return false
}

func containsRustlsRead(name string) bool {
	patterns := []string{
		"rustls::connection::Connection::read_tls",
		"rustls::conn::Connection::read_tls",
		"_ZN6rustls10connection10Connection8read_tls",
	}
	for _, p := range patterns {
		if name == p {
			return true
		}
	}
	return false
}

func scanTextSection(exe *elf.File) (*RustlsOffsets, error) {
	textSection := exe.Section(".text")
	if textSection == nil {
		return nil, fmt.Errorf(".text section not found")
	}

	data, err := textSection.Data()
	if err != nil {
		return nil, fmt.Errorf("read .text: %w", err)
	}

	// rustls write_tls 特征：
	// - 调用 writev/write 系统调用前的准备
	// - 参数处理模式：&mut self, writer
	writeOffset := findRustlsWritePattern(data, textSection.Addr)
	readOffset := findRustlsReadPattern(data, textSection.Addr)

	if writeOffset == 0 && readOffset == 0 {
		return nil, fmt.Errorf("rustls functions not found")
	}

	return &RustlsOffsets{
		WriteTLS: writeOffset,
		ReadTLS:  readOffset,
	}, nil
}

// findRustlsWritePattern 查找 write_tls 的字节码模式
func findRustlsWritePattern(data []byte, baseAddr uint64) uint64 {
	// 模式：函数序言 + 特征调用
	// rustls write_tls 会调用 io::Write trait 方法
	patterns := [][]byte{
		// x86_64: push rbp; mov rbp, rsp
		{0x55, 0x48, 0x89, 0xe5},
		// x86_64: sub rsp, N (栈帧分配)
		{0x48, 0x83, 0xec},
	}

	for i := 0; i < len(data)-100; i++ {
		if matchesPattern(data[i:], patterns[0]) {
			// 检查后续是否有栈分配
			if i+10 < len(data) && matchesPattern(data[i+4:], patterns[1][:2]) {
				// 进一步验证：检查是否有 writev/write 系统调用特征
				if hasWriteSyscallNearby(data, i) {
					return baseAddr + uint64(i)
				}
			}
		}
	}
	return 0
}

func findRustlsReadPattern(data []byte, baseAddr uint64) uint64 {
	patterns := [][]byte{
		{0x55, 0x48, 0x89, 0xe5},
		{0x48, 0x83, 0xec},
	}

	for i := 0; i < len(data)-100; i++ {
		if matchesPattern(data[i:], patterns[0]) {
			if i+10 < len(data) && matchesPattern(data[i+4:], patterns[1][:2]) {
				if hasReadSyscallNearby(data, i) {
					return baseAddr + uint64(i)
				}
			}
		}
	}
	return 0
}

func matchesPattern(data []byte, pattern []byte) bool {
	if len(data) < len(pattern) {
		return false
	}
	for i := range pattern {
		if data[i] != pattern[i] {
			return false
		}
	}
	return true
}

func hasWriteSyscallNearby(data []byte, offset int) bool {
	// 查找范围：函数开始后 200 字节内
	end := offset + 200
	if end > len(data) {
		end = len(data)
	}

	// 查找 writev/write 系统调用号或调用指令
	for i := offset; i < end-8; i++ {
		// syscall 指令：0x0f 0x05
		if data[i] == 0x0f && data[i+1] == 0x05 {
			// 检查之前是否设置了 rax（syscall number）
			// write=1, writev=20
			if hasSyscallNumber(data[offset:i], 1) || hasSyscallNumber(data[offset:i], 20) {
				return true
			}
		}
	}
	return false
}

func hasReadSyscallNearby(data []byte, offset int) bool {
	end := offset + 200
	if end > len(data) {
		end = len(data)
	}

	for i := offset; i < end-8; i++ {
		if data[i] == 0x0f && data[i+1] == 0x05 {
			// read=0, readv=19
			if hasSyscallNumber(data[offset:i], 0) || hasSyscallNumber(data[offset:i], 19) {
				return true
			}
		}
	}
	return false
}

func hasSyscallNumber(data []byte, num uint32) bool {
	// 查找 mov $num, %eax 或 mov $num, %rax
	// mov eax, imm32: b8 [4 bytes]
	// mov rax, imm32: 48 c7 c0 [4 bytes]

	for i := 0; i < len(data)-5; i++ {
		// mov eax, imm32
		if data[i] == 0xb8 {
			val := binary.LittleEndian.Uint32(data[i+1:])
			if val == num {
				return true
			}
		}
		// mov rax, imm32
		if i < len(data)-7 && data[i] == 0x48 && data[i+1] == 0xc7 && data[i+2] == 0xc0 {
			val := binary.LittleEndian.Uint32(data[i+3:])
			if val == num {
				return true
			}
		}
	}
	return false
}
