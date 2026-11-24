package app

import "agent-ebpf-filter/app/network"

func syscallName(nr uint32) string {
	return network.SyscallName(nr)
}

func syscallNameExtended(nr uint32) string {
	return network.SyscallNameExtended(nr)
}

func syscallNameHigh(nr uint32) string {
	return network.SyscallNameHigh(nr)
}
