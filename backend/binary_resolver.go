package main

import "agent-ebpf-filter/internal/binaryresolver"

type ResolvedBinary = binaryresolver.ResolvedBinary

func ResolveBinary(input string, envPath string) ResolvedBinary {
	return binaryresolver.ResolveBinary(input, envPath)
}
