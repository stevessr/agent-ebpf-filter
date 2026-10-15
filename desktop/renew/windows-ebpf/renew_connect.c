// SPDX-License-Identifier: MIT
// eBPF for Windows, not a Linux tracepoint program.
// Count outbound connection hook invocations. Always soft-permit: no blocking.
#include <stdint.h>
#include "bpf_helpers.h"

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __type(key, uint32_t);
    __type(value, uint64_t);
    __uint(max_entries, 2);
} renew_conn SEC(".maps");

static __inline void count_connect(uint32_t index) {
    uint64_t *counter = bpf_map_lookup_elem(&renew_conn, &index);
    if (counter != NULL) {
        __sync_fetch_and_add(counter, 1);
    }
}

SEC("cgroup/connect4")
int renew_connect4(bpf_sock_addr_t *ctx) {
    (void)ctx;
    count_connect(0);
    return BPF_SOCK_ADDR_VERDICT_PROCEED_SOFT;
}

SEC("cgroup/connect6")
int renew_connect6(bpf_sock_addr_t *ctx) {
    (void)ctx;
    count_connect(1);
    return BPF_SOCK_ADDR_VERDICT_PROCEED_SOFT;
}

char _license[] SEC("license") = "MIT";
