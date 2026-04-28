> Local snapshot: Linux 6.18 LTS
> Source: https://docs.ebpf.io/linux/helper-function/bpf_ktime_get_ns/
> Cached: 2026-04-28

[Skip to content](#helper-function-bpf_ktime_get_ns)



* [Definition](#definition) 

  + [Returns](#returns)
* [Usage](#usage) 

  + [Program types](#program-types)
  + [Example](#example)

# Helper function `bpf_ktime_get_ns`

[v4.1](https://github.com/torvalds/linux/commit/d9847d310ab4003725e6ed1822682e24bd406908)

## Definition

> Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Return the time elapsed since system boot, in nanoseconds. Does not include time the system was suspended. See: **clock\_gettime**(**CLOCK\_MONOTONIC**)

### Returns

Current *ktime*.

`static __u64 (* const bpf_ktime_get_ns)(void) = (void *) 5;`

## Usage

Returns a 64-bit value representing the current kernel time in nanoseconds since the system boot, excluding any time the system was suspended. This can be useful for measuring time intervals or generating timestamps in eBPF programs.

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
* [`BPF_PROG_TYPE_LIRC_MODE2`](../../program-type/BPF_PROG_TYPE_LIRC_MODE2/)
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
 __u64  start_time  =  bpf_ktime_get_ns();/* some tasks */ __u64  end_time  =  bpf_ktime_get_ns(); __u64  duration  =  end_time  -  start_time;
```
