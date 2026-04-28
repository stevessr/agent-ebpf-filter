> Local snapshot: Linux 6.18 LTS
> Source: https://docs.ebpf.io/linux/helper-function/bpf_ringbuf_reserve/
> Cached: 2026-04-28

[Skip to content](#helper-function-bpf_ringbuf_reserve)



* [Definition](#definition) 

  + [Returns](#returns)
* [Usage](#usage) 

  + [Program types](#program-types)
  + [Example](#example)

# Helper function `bpf_ringbuf_reserve`

[v5.8](https://github.com/torvalds/linux/commit/457f44363a8894135c85b7a9afd2bd8196db24ab)

## Definition

> Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Reserve *size* bytes of payload in a ring buffer *ringbuf*. *flags* must be 0.

### Returns

Valid pointer with *size* bytes of memory available; NULL, otherwise.

`static void *(* const bpf_ringbuf_reserve)(void * ringbuf, __u64 size, __u64 flags) = (void *) 131;`

## Usage

The `ringbuf` argument must be a pointer to a ring buffer definition. The `size` argument specifies the number of bytes to be reserved in the ring buffer. And the `flags` argument must be set to 0.

This function is generally used in combination with a `struct` that defines the structure of the data stored in the ring buffer. Hence, in this case, the `size` argument would be set to the size of the struct. The function returns a pointer to the reserved memory, which can be used to write data to the ring buffer. See the example below for more details.

The verifier enforces the constraint that for every call to `bpf_ringbuf_reserve`, a subsequent [`bpf_ringbuf_submit`](../bpf_ringbuf_submit/) or [`bpf_ringbuf_discard`](../bpf_ringbuf_discard/) must be called. Check [`bpf_ringbuf_submit`](../bpf_ringbuf_submit/) and [`bpf_ringbuf_discard`](../bpf_ringbuf_discard/) for more information.

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
 struct  ringbuf_data  * rb_data  =  bpf_ringbuf_reserve(& my_ringbuf,  sizeof(struct  ringbuf_data),  0); if  (!  rb_data)  { // if bpf_ringbuf_reserve fails, print an error message and return  bpf_printk("bpf_ringbuf_reserve failed \n ");  return  1;}
```

Where `my_ringbuf` is the pointer to the ring buffer, and `ringbuf_data` is a struct that defines the structure of the data to be stored in the ring buffer.
