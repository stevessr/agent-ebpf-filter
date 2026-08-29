package tls

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
)

const strippedSSLScanChunkSize int64 = 1 << 20

type strippedSSLFileRange struct {
	offset int64
	size   int64
}

func executableFileRanges(file *elf.File) []strippedSSLFileRange {
	if file == nil {
		return nil
	}
	ranges := make([]strippedSSLFileRange, 0, len(file.Progs))
	for _, program := range file.Progs {
		if program == nil || program.Type != elf.PT_LOAD || program.Flags&elf.PF_X == 0 || program.Filesz == 0 {
			continue
		}
		ranges = append(ranges, strippedSSLFileRange{
			offset: int64(program.Off),
			size:   int64(program.Filesz),
		})
	}
	return ranges
}

func scanMaskedFileRange(
	file *os.File,
	rangeOffset int64,
	rangeSize int64,
	pattern []byte,
	mask []byte,
	visit func(int64) bool,
) error {
	if file == nil || rangeSize <= 0 || len(pattern) == 0 || visit == nil {
		return nil
	}
	if mask == nil {
		mask = make([]byte, len(pattern))
		for index := range mask {
			mask[index] = 0xff
		}
	}
	if len(mask) != len(pattern) {
		return fmt.Errorf("scan pattern mask length %d does not match pattern length %d", len(mask), len(pattern))
	}

	end := rangeOffset + rangeSize
	if end < rangeOffset {
		return fmt.Errorf("scan range overflows file offset space")
	}
	buffer := make([]byte, strippedSSLScanChunkSize+int64(len(pattern))-1)
	for chunkStart := rangeOffset; chunkStart < end; chunkStart += strippedSSLScanChunkSize {
		primary := strippedSSLScanChunkSize
		if remaining := end - chunkStart; remaining < primary {
			primary = remaining
		}
		readLength := primary + int64(len(pattern)) - 1
		if remaining := end - chunkStart; remaining < readLength {
			readLength = remaining
		}
		window := buffer[:int(readLength)]
		n, err := file.ReadAt(window, chunkStart)
		if err != nil && err != io.EOF {
			return fmt.Errorf("scan file at offset %#x: %w", chunkStart, err)
		}
		window = window[:n]
		maxStarts := int(primary)
		if maxStarts > len(window) {
			maxStarts = len(window)
		}
		for index := 0; index < maxStarts && index+len(pattern) <= len(window); index++ {
			if matchMasked(window[index:], pattern, mask) && !visit(chunkStart+int64(index)) {
				return nil
			}
		}
		if int64(n) < readLength {
			return fmt.Errorf("short read while scanning file at offset %#x: got %d bytes, expected %d", chunkStart, n, readLength)
		}
	}
	return nil
}

func firstMaskedFileMatch(file *os.File, ranges []strippedSSLFileRange, pattern, mask []byte) (int64, error) {
	match := int64(-1)
	for _, fileRange := range ranges {
		err := scanMaskedFileRange(file, fileRange.offset, fileRange.size, pattern, mask, func(offset int64) bool {
			match = offset
			return false
		})
		if err != nil {
			return -1, err
		}
		if match >= 0 {
			return match, nil
		}
	}
	return -1, nil
}

func firstExactFileMatch(file *os.File, ranges []strippedSSLFileRange, pattern []byte) (int64, error) {
	mask := make([]byte, len(pattern))
	for index := range mask {
		mask[index] = 0xff
	}
	return firstMaskedFileMatch(file, ranges, pattern, mask)
}

func firstExactFileMatchNear(
	file *os.File,
	ranges []strippedSSLFileRange,
	pattern []byte,
	center int64,
	searchRange int64,
) (int64, error) {
	windowStart := center - searchRange
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := center + searchRange
	for _, fileRange := range ranges {
		start := fileRange.offset
		end := fileRange.offset + fileRange.size
		if start < windowStart {
			start = windowStart
		}
		if end > windowEnd {
			end = windowEnd
		}
		if end <= start {
			continue
		}
		match, err := firstExactFileMatch(file, []strippedSSLFileRange{{offset: start, size: end - start}}, pattern)
		if err != nil {
			return -1, err
		}
		if match >= 0 {
			return match, nil
		}
	}
	return -1, nil
}

func countMaskedFileMatches(file *os.File, ranges []strippedSSLFileRange, pattern, mask []byte) ([]int64, int, error) {
	firstTwo := make([]int64, 0, 2)
	count := 0
	for _, fileRange := range ranges {
		err := scanMaskedFileRange(file, fileRange.offset, fileRange.size, pattern, mask, func(offset int64) bool {
			count++
			if len(firstTwo) < 2 {
				firstTwo = append(firstTwo, offset)
			}
			return true
		})
		if err != nil {
			return nil, 0, err
		}
	}
	return firstTwo, count, nil
}

func fileContainsPattern(file *os.File, fileSize int64, pattern []byte) (bool, error) {
	match, err := firstExactFileMatch(file, []strippedSSLFileRange{{offset: 0, size: fileSize}}, pattern)
	return match >= 0, err
}

func discoverKnownStrippedSSLFile(path string) (strippedSSLDiscovery, error) {
	raw, err := os.Open(path)
	if err != nil {
		return strippedSSLDiscovery{}, fmt.Errorf("open stripped SSL target %q: %w", path, err)
	}
	defer raw.Close()

	info, err := raw.Stat()
	if err != nil {
		return strippedSSLDiscovery{}, fmt.Errorf("stat stripped SSL target %q: %w", path, err)
	}
	parsed, err := elf.NewFile(raw)
	if err != nil {
		return strippedSSLDiscovery{}, fmt.Errorf("parse stripped SSL target %q: %w", path, err)
	}
	defer parsed.Close()
	if parsed.Machine != elf.EM_X86_64 {
		return strippedSSLDiscovery{}, fmt.Errorf(
			"safe stripped SSL offsets are unavailable for ELF machine %s",
			parsed.Machine,
		)
	}

	executable := executableFileRanges(parsed)
	if len(executable) == 0 {
		return strippedSSLDiscovery{}, fmt.Errorf("ELF target %q has no executable PT_LOAD segment", path)
	}

	readOff, err := firstExactFileMatch(raw, executable, bsSSLRead.pattern)
	if err != nil {
		return strippedSSLDiscovery{}, err
	}
	if readOff >= 0 {
		writeOff, err := firstExactFileMatchNear(raw, executable, bsSSLWrite.pattern, readOff+0xCA0, 0x10000)
		if err != nil {
			return strippedSSLDiscovery{}, err
		}
		if writeOff >= 0 {
			return strippedSSLDiscovery{
				ReadOffset:  uint64(readOff),
				WriteOffset: uint64(writeOff),
				Strategy:    strippedSSLStrategyBoringSSLExact,
			}, nil
		}
	}

	hasReadName, err := fileContainsPattern(raw, info.Size(), []byte("SSL_read"))
	if err != nil {
		return strippedSSLDiscovery{}, err
	}
	hasWriteName, err := fileContainsPattern(raw, info.Size(), []byte("SSL_write"))
	if err != nil {
		return strippedSSLDiscovery{}, err
	}
	if !hasReadName || !hasWriteName {
		return strippedSSLDiscovery{}, fmt.Errorf(
			"no exact BoringSSL match and binary does not contain both SSL_read and SSL_write names",
		)
	}

	mask := buildMask(osslSSLCommonPrefix.pattern)
	firstTwo, candidateCount, err := countMaskedFileMatches(raw, executable, osslSSLCommonPrefix.pattern, mask)
	if err != nil {
		return strippedSSLDiscovery{}, err
	}
	writeOff, readOff, _, ok := selectUnambiguousOpenSSLPair(firstTwo, candidateCount == 2)
	if !ok {
		return strippedSSLDiscovery{}, fmt.Errorf(
			"no unambiguous stripped SSL_read/SSL_write offsets (%d common-prefix candidates)",
			candidateCount,
		)
	}
	return strippedSSLDiscovery{
		ReadOffset:     uint64(readOff),
		WriteOffset:    uint64(writeOff),
		Strategy:       strippedSSLStrategyOpenSSLCommonPrefix,
		CandidateCount: candidateCount,
	}, nil
}
