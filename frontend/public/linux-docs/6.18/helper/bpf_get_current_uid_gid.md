> Local snapshot: Linux 6.18 LTS
> Source: https://docs.ebpf.io/linux/helper-function/bpf_get_current_uid_gid/
> Cached: 2026-04-28

[Skip to content](#helper-function-bpf_get_current_uid_gid)



* [Definition](#definition) 

  + [Returns](#returns)
* [Usage](#usage) 

  + [Program types](#program-types)
  + [Example](#example)

# Helper function `bpf_get_current_uid_gid`

[v4.2](https://github.com/torvalds/linux/commit/ffeedafbf0236f03aeb2e8db273b3e5ae5f5bc89)

## Definition

> Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Get the current uid and gid.

### Returns

A 64-bit integer containing the current GID and UID, and created as such: *current\_gid* **<< 32 |** *current\_uid*.

`static __u64 (* const bpf_get_current_uid_gid)(void) = (void *) 15;`

## Usage

The `bpf_get_current_uid_gid` helper function returns a 64-bit value containing the current task's UID in the lower 32 bits and GID in the upper 32 bits. This allows eBPF programs to identify the user and group context of the running task. It is useful for enforcing security policies, tracking actions by specific users or groups, and implementing per-UID or per-GID tracing.

### Program types

This helper call can be used in the following program types:

* [`BPF_PROG_TYPE_CGROUP_DEVICE`](../../program-type/BPF_PROG_TYPE_CGROUP_DEVICE/)
* [`BPF_PROG_TYPE_CGROUP_SOCK`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCK/)
* [`BPF_PROG_TYPE_CGROUP_SOCKOPT`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCKOPT/)
* [`BPF_PROG_TYPE_CGROUP_SOCK_ADDR`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCK_ADDR/)
* [`BPF_PROG_TYPE_CGROUP_SYSCTL`](../../program-type/BPF_PROG_TYPE_CGROUP_SYSCTL/)
* [`BPF_PROG_TYPE_KPROBE`](../../program-type/BPF_PROG_TYPE_KPROBE/)
* [`BPF_PROG_TYPE_LSM`](../../program-type/BPF_PROG_TYPE_LSM/)
* [`BPF_PROG_TYPE_PERF_EVENT`](../../program-type/BPF_PROG_TYPE_PERF_EVENT/)
* [`BPF_PROG_TYPE_RAW_TRACEPOINT`](../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT/)
* [`BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE`](../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE/)
* [`BPF_PROG_TYPE_SK_MSG`](../../program-type/BPF_PROG_TYPE_SK_MSG/)
* [`BPF_PROG_TYPE_SYSCALL`](../../program-type/BPF_PROG_TYPE_SYSCALL/)
* [`BPF_PROG_TYPE_TRACEPOINT`](../../program-type/BPF_PROG_TYPE_TRACEPOINT/)
* [`BPF_PROG_TYPE_TRACING`](../../program-type/BPF_PROG_TYPE_TRACING/)

### Example

```
 #include   #include   SEC("tp/syscalls/sys_enter_open") int  sys_open_trace(void  * ctx)  { __u64  uid_gid  =  bpf_get_current_uid_gid();  __u32  uid  =  uid_gid  &  0xFFFFFFFF;  __u32  gid  =  uid_gid  >>  32;  bpf_printk("Hello from UID %u, GID %u \n ",  uid,  gid);  return  0;}
```
