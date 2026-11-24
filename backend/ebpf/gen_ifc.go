package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -type ifc_task_label -type ifc_inode_label -type ifc_inode_key -type ifc_stats AgentIfcEnforcer ifc_enforcer.c -- -I. -I/usr/include/x86_64-linux-gnu -I/usr/include/aarch64-linux-gnu -Wno-missing-declarations
