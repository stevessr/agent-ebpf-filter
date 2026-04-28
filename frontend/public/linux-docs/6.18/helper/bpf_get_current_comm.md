> Local snapshot: Linux 6.18 LTS
> Source: https://docs.ebpf.io/linux/helper-function/bpf_get_current_comm/
> Cached: 2026-04-28

![logo](../../../assets/image/logo.png)
![logo](../../../assets/image/logo.png)

# Helper function `bpf_get_current_comm`

`bpf_get_current_comm`

[v4.2](https://github.com/torvalds/linux/commit/ffeedafbf0236f03aeb2e8db273b3e5ae5f5bc89)

## Definition

Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Copy the **comm** attribute of the current task into *buf* of *size\_of\_buf*. The **comm** attribute contains the name of the executable (excluding the path) for the current task. The *size\_of\_buf* must be strictly positive. On success, the helper makes sure that the *buf* is NUL-terminated. On failure, it is filled with zeroes.

### Returns

0 on success, or a negative error in case of failure.

`static long (* const bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *) 16;`

`static long (* const bpf_get_current_comm)(void *buf, __u32 size_of_buf) = (void *) 16;`

## Usage

The `bpf_get_current_comm` helper function retrieves the name of the executable associated with the current task. This is useful for identifying the process context in which the eBPF program is executing, enabling per-process tracing. It can help trace specific applications, enforce process-level policies, or monitor system behavior tied to particular commands.

`bpf_get_current_comm`

### Program types

This helper call can be used in the following program types:

`BPF_PROG_TYPE_CGROUP_SOCK`
`BPF_PROG_TYPE_CGROUP_SOCK_ADDR`
`BPF_PROG_TYPE_KPROBE`
`BPF_PROG_TYPE_LSM`
`BPF_PROG_TYPE_PERF_EVENT`
`BPF_PROG_TYPE_RAW_TRACEPOINT`
`BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE`
`BPF_PROG_TYPE_SYSCALL`
`BPF_PROG_TYPE_TRACEPOINT`
`BPF_PROG_TYPE_TRACING`

### Example

`#include <vmlinux.h>
#include <bpf/bpf_helpers.h>
SEC("tp/syscalls/sys_enter_open")
int sys_open_trace(void *ctx) {
 // TASK_COMM_LEN is defined in vmlinux.h
 char comm[TASK_COMM_LEN];
 if (bpf_get_current_comm(comm, TASK_COMM_LEN)) {
 bpf_printk("Failed to get comm\n");
 return 0;
 }
 bpf_printk("Hello from %s\n", comm);
 return 0;
}`
![jetlime](https://avatars.githubusercontent.com/u/29337128?v=4&size=72)
![Andreagit97](https://avatars.githubusercontent.com/u/66700518?v=4&size=72)
![dylandreimerink](https://avatars.githubusercontent.com/u/1799415?v=4&size=72)
![dkanaliev](https://avatars.githubusercontent.com/u/19514094?v=4&size=72)
