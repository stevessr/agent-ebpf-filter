> Local snapshot: Linux 6.18 LTS
> Source: https://docs.ebpf.io/linux/helper-function/bpf_get_stackid/
> Cached: 2026-04-28

![logo](../../../assets/image/logo.png)
![logo](../../../assets/image/logo.png)

# Helper function `bpf_get_stackid`

`bpf_get_stackid`

[v4.6](https://github.com/torvalds/linux/commit/d5a3b1f691865be576c2bffa708549b8cdccda19)

## Definition

Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Walk a user or a kernel stack and return its id. To achieve this, the helper needs *ctx*, which is a pointer to the context on which the tracing program is executed, and a pointer to a *map* of type **BPF\_MAP\_TYPE\_STACK\_TRACE**.

The last argument, *flags*, holds the number of stack frames to skip (from 0 to 255), masked with **BPF\_F\_SKIP\_FIELD\_MASK**. The next bits can be used to set a combination of the following flags:

**BPF\_F\_USER\_STACK**

    Collect a user space stack instead of a kernel stack.

**BPF\_F\_FAST\_STACK\_CMP**

    Compare stacks by hash only.

**BPF\_F\_REUSE\_STACKID**

    If two different stacks hash into the same *stackid*, discard the old one.

The stack id retrieved is a 32 bit long integer handle which can be further combined with other data (including other stack ids) and used as a key into maps. This can be useful for generating a variety of graphs (such as flame graphs or off-cpu graphs).

For walking a stack, this helper is an improvement over **bpf\_probe\_read**(), which can be used with unrolled loops but is not efficient and consumes a lot of eBPF instructions. Instead, **bpf\_get\_stackid**() can collect up to **PERF\_MAX\_STACK\_DEPTH** both kernel and user frames. Note that this limit can be controlled with the **sysctl** program, and that it should be manually increased in order to profile long user stacks (such as stacks for Java programs). To do so, use:

`# sysctl kernel.perf_event_max_stack=<new value>`

### Returns

The positive or null stack id on success, or a negative error in case of failure.

`static long (* const bpf_get_stackid)(void *ctx, void *map, __u64 flags) = (void *) 27;`

`static long (* const bpf_get_stackid)(void *ctx, void *map, __u64 flags) = (void *) 27;`

## Usage

Call `bpf_get_stackid` to retrieve the stack id of the context in which the program is running, specifying as arguments:

`bpf_get_stackid`
`BPF_MAP_TYPE_STACK_TRACE`
`long bpf_get_stackid(void *ctx, struct bpf_map *map, u64 flags)`

### Program types

This helper call can be used in the following program types:

`BPF_PROG_TYPE_KPROBE`
`BPF_PROG_TYPE_PERF_EVENT`
`BPF_PROG_TYPE_RAW_TRACEPOINT`
`BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE`
`BPF_PROG_TYPE_TRACEPOINT`
`BPF_PROG_TYPE_TRACING`

### Example

`#include <bpf/bpf_helpers.h>
struct {
 __uint(type, BPF_MAP_TYPE_STACK_TRACE);
 __uint(key_size, sizeof(u32));
 __uint(value_size, PERF_MAX_STACK_DEPTH * sizeof(u64));
 __uint(max_entries, 10000);
} stack_traces SEC(".maps");
SEC("perf_event")
int print_stack_ids(struct bpf_perf_event_data *ctx)
{
 char fmt[] = "kern_stack_id=%d user_stack_id=%d";
 kern_stack_id = bpf_get_stackid(ctx, &stack_traces, 0);
 user_stack_id = bpf_get_stackid(ctx, &stack_traces, 0 | BPF_F_USER_STACK);
 if kern_stack_id >= 0 && user_stack_id >=0 {
 bpf_trace_printk(fmt, sizeof(fmt), kern_stack_id, user_stack_id);
 }
}
char _license[] SEC("license") = "GPL";`

Complete examples in the Linux source bpf samples:

`samples/bpf/offwaketime.bpf.c`
`samples/bpf/trace_event_kern.c`
![dylandreimerink](https://avatars.githubusercontent.com/u/1799415?v=4&size=72)
![dkanaliev](https://avatars.githubusercontent.com/u/19514094?v=4&size=72)
