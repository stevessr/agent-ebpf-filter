> Local snapshot: Linux 6.18 LTS
> Source: https://docs.ebpf.io/linux/helper-function/bpf_get_smp_processor_id/
> Cached: 2026-04-28

[Skip to content](#helper-function-bpf_get_smp_processor_id)



* [Definition](#definition)

  + [Returns](#returns)
* [Usage](#usage)

  + [Program types](#program-types)
  + [Example](#example)

# Helper function `bpf_get_smp_processor_id`

[v4.1](https://github.com/torvalds/linux/commit/c04167ce2ca0ecaeaafef006cb0d65cf01b68e42)

## Definition

> Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Get the SMP (symmetric multiprocessing) processor id. Note that all programs run with migration disabled, which means that the SMP processor id is stable during all the execution of the program.

### Returns

The SMP id of the processor running the program.

`static __bpf_fastcall __u32 (* const bpf_get_smp_processor_id)(void) = (void *) 8;`

## Usage

The `bpf_get_smp_processor_id` helper function returns a 32-bit value, containing the id of the current SMP (symmetric multiprocessing) processor executing the program. This helper function allows eBPF programs to identify the processor id, which can be useful for performance monitoring or debugging.

### Program types

This helper call can be used in the following program types:



* [`BPF_PROG_TYPE_CGROUP_DEVICE`](../../program-type/BPF_PROG_TYPE_CGROUP_DEVICE/)
* [`BPF_PROG_TYPE_CGROUP_SKB`](../../program-type/BPF_PROG_TYPE_CGROUP_SKB/)
* [`BPF_PROG_TYPE_CGROUP_SOCK`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCK/)
* [`BPF_PROG_TYPE_CGROUP_SOCKOPT`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCKOPT/)
* [`BPF_PROG_TYPE_CGROUP_SOCK_ADDR`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCK_ADDR/)
* [`BPF_PROG_TYPE_CGROUP_SYSCTL`](../../program-type/BPF_PROG_TYPE_CGROUP_SYSCTL/)
* [`BPF_PROG_TYPE_FLOW_DISSECTOR`](../../program-type/BPF_PROG_TYPE_FLOW_DISSECTOR/)
* [`BPF_PROG_TYPE_KPROBE`](../../program-type/BPF_PROG_TYPE_KPROBE/)
* [`BPF_PROG_TYPE_LSM`](../../program-type/BPF_PROG_TYPE_LSM/)
* [`BPF_PROG_TYPE_LWT_IN`](../../program-type/BPF_PROG_TYPE_LWT_IN/)
* [`BPF_PROG_TYPE_LWT_OUT`](../../program-type/BPF_PROG_TYPE_LWT_OUT/)
* [`BPF_PROG_TYPE_LWT_SEG6LOCAL`](../../program-type/BPF_PROG_TYPE_LWT_SEG6LOCAL/)
* [`BPF_PROG_TYPE_LWT_XMIT`](../../program-type/BPF_PROG_TYPE_LWT_XMIT/)
* [`BPF_PROG_TYPE_NETFILTER`](../../program-type/BPF_PROG_TYPE_NETFILTER/)
* [`BPF_PROG_TYPE_PERF_EVENT`](../../program-type/BPF_PROG_TYPE_PERF_EVENT/)
* [`BPF_PROG_TYPE_RAW_TRACEPOINT`](../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT/)
* [`BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE`](../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE/)
* [`BPF_PROG_TYPE_SCHED_ACT`](../../program-type/BPF_PROG_TYPE_SCHED_ACT/)
* [`BPF_PROG_TYPE_SCHED_CLS`](../../program-type/BPF_PROG_TYPE_SCHED_CLS/)
* [`BPF_PROG_TYPE_SK_LOOKUP`](../../program-type/BPF_PROG_TYPE_SK_LOOKUP/)
* [`BPF_PROG_TYPE_SK_MSG`](../../program-type/BPF_PROG_TYPE_SK_MSG/)
* [`BPF_PROG_TYPE_SK_REUSEPORT`](../../program-type/BPF_PROG_TYPE_SK_REUSEPORT/)
* [`BPF_PROG_TYPE_SK_SKB`](../../program-type/BPF_PROG_TYPE_SK_SKB/)
* [`BPF_PROG_TYPE_SOCKET_FILTER`](../../program-type/BPF_PROG_TYPE_SOCKET_FILTER/)
* [`BPF_PROG_TYPE_SOCK_OPS`](../../program-type/BPF_PROG_TYPE_SOCK_OPS/)
* [`BPF_PROG_TYPE_STRUCT_OPS`](../../program-type/BPF_PROG_TYPE_STRUCT_OPS/)
* [`BPF_PROG_TYPE_SYSCALL`](../../program-type/BPF_PROG_TYPE_SYSCALL/)
* [`BPF_PROG_TYPE_TRACEPOINT`](../../program-type/BPF_PROG_TYPE_TRACEPOINT/)
* [`BPF_PROG_TYPE_TRACING`](../../program-type/BPF_PROG_TYPE_TRACING/)
* [`BPF_PROG_TYPE_XDP`](../../program-type/BPF_PROG_TYPE_XDP/)

### Example

```
#include <vmlinux.h>
#include <bpf/bpf_helpers.h>

SEC("tp/syscalls/sys_enter_open")
int sys_open_trace(void *ctx) {
    __u32 processor = bpf_get_smp_processor_id();
    bpf_printk("Executed on processor %u.\n", processor);
    return 0;
}

```
