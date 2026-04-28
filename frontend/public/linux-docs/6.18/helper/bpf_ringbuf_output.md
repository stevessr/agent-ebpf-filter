> Local snapshot: Linux 6.18 LTS (HTML fallback)
> Source: https://docs.ebpf.io/linux/helper-function/bpf_ringbuf_output/
> Cached: 2026-04-28

<div class="md-main" role="main" md-component="main">

<div class="md-main__inner md-grid">

<div class="md-sidebar md-sidebar--primary" md-component="sidebar" md-type="navigation">

<div class="md-sidebar__scrollwrap">

<div class="md-sidebar__inner">

<a href="../../.." class="md-nav__button md-logo" aria-label="eBPF Docs" data-md-component="logo" title="eBPF Docs"><img src="../../../assets/image/logo.png" alt="logo" /></a> eBPF Docs

<div class="md-nav__source">

<a href="https://github.com/isovalent/ebpf-docs" class="md-source" data-md-component="source" title="Go to repository"></a>

<div class="md-source__icon md-icon">

![](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCA0OTYgNTEyIj48IS0tISBGb250IEF3ZXNvbWUgRnJlZSA2LjYuMCBieSBAZm9udGF3ZXNvbWUgLSBodHRwczovL2ZvbnRhd2Vzb21lLmNvbSBMaWNlbnNlIC0gaHR0cHM6Ly9mb250YXdlc29tZS5jb20vbGljZW5zZS9mcmVlIChJY29uczogQ0MgQlkgNC4wLCBGb250czogU0lMIE9GTCAxLjEsIENvZGU6IE1JVCBMaWNlbnNlKSBDb3B5cmlnaHQgMjAyNCBGb250aWNvbnMsIEluYy4tLT48cGF0aCBkPSJNMTY1LjkgMzk3LjRjMCAyLTIuMyAzLjYtNS4yIDMuNi0zLjMuMy01LjYtMS4zLTUuNi0zLjYgMC0yIDIuMy0zLjYgNS4yLTMuNiAzLS4zIDUuNiAxLjMgNS42IDMuNm0tMzEuMS00LjVjLS43IDIgMS4zIDQuMyA0LjMgNC45IDIuNiAxIDUuNiAwIDYuMi0ycy0xLjMtNC4zLTQuMy01LjJjLTIuNi0uNy01LjUuMy02LjIgMi4zbTQ0LjItMS43Yy0yLjkuNy00LjkgMi42LTQuNiA0LjkuMyAyIDIuOSAzLjMgNS45IDIuNiAyLjktLjcgNC45LTIuNiA0LjYtNC42LS4zLTEuOS0zLTMuMi01LjktMi45TTI0NC44IDhDMTA2LjEgOCAwIDExMy4zIDAgMjUyYzAgMTEwLjkgNjkuOCAyMDUuOCAxNjkuNSAyMzkuMiAxMi44IDIuMyAxNy4zLTUuNiAxNy4zLTEyLjEgMC02LjItLjMtNDAuNC0uMy02MS40IDAgMC03MCAxNS04NC43LTI5LjggMCAwLTExLjQtMjkuMS0yNy44LTM2LjYgMCAwLTIyLjktMTUuNyAxLjYtMTUuNCAwIDAgMjQuOSAyIDM4LjYgMjUuOCAyMS45IDM4LjYgNTguNiAyNy41IDcyLjkgMjAuOSAyLjMtMTYgOC44LTI3LjEgMTYtMzMuNy01NS45LTYuMi0xMTIuMy0xNC4zLTExMi4zLTExMC41IDAtMjcuNSA3LjYtNDEuMyAyMy42LTU4LjktMi42LTYuNS0xMS4xLTMzLjMgMi42LTY3LjkgMjAuOS02LjUgNjkgMjcgNjkgMjcgMjAtNS42IDQxLjUtOC41IDYyLjgtOC41czQyLjggMi45IDYyLjggOC41YzAgMCA0OC4xLTMzLjYgNjktMjcgMTMuNyAzNC43IDUuMiA2MS40IDIuNiA2Ny45IDE2IDE3LjcgMjUuOCAzMS41IDI1LjggNTguOSAwIDk2LjUtNTguOSAxMDQuMi0xMTQuOCAxMTAuNSA5LjIgNy45IDE3IDIyLjkgMTcgNDYuNCAwIDMzLjctLjMgNzUuNC0uMyA4My42IDAgNi41IDQuNiAxNC40IDE3LjMgMTIuMUM0MjguMiA0NTcuOCA0OTYgMzYyLjkgNDk2IDI1MiA0OTYgMTEzLjMgMzgzLjUgOCAyNDQuOCA4TTk3LjIgMzUyLjljLTEuMyAxLTEgMy4zLjcgNS4yIDEuNiAxLjYgMy45IDIuMyA1LjIgMSAxLjMtMSAxLTMuMy0uNy01LjItMS42LTEuNi0zLjktMi4zLTUuMi0xbS0xMC44LTguMWMtLjcgMS4zLjMgMi45IDIuMyAzLjkgMS42IDEgMy42LjcgNC4zLS43LjctMS4zLS4zLTIuOS0yLjMtMy45LTItLjYtMy42LS4zLTQuMy43bTMyLjQgMzUuNmMtMS42IDEuMy0xIDQuMyAxLjMgNi4yIDIuMyAyLjMgNS4yIDIuNiA2LjUgMSAxLjMtMS4zLjctNC4zLTEuMy02LjItMi4yLTIuMy01LjItMi42LTYuNS0xbS0xMS40LTE0LjdjLTEuNiAxLTEuNiAzLjYgMCA1LjlzNC4zIDMuMyA1LjYgMi4zYzEuNi0xLjMgMS42LTMuOSAwLTYuMi0xLjQtMi4zLTQtMy4zLTUuNi0yIiAvPjwvc3ZnPg==)

</div>

<div class="md-source__repository">

GitHub

</div>

</div>

<a href="../../.." class="md-nav__link"><span class="md-ellipsis"> Home </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../" class="md-nav__link"><span class="md-ellipsis"> Linux Reference </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Linux Reference

<div class="md-nav__link md-nav__container">

<a href="../../concepts/" class="md-nav__link"><span class="md-ellipsis"> Concepts </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Concepts

- <a href="../../concepts/maps/" class="md-nav__link"><span class="md-ellipsis"> Maps </span></a>
- <a href="../../concepts/verifier/" class="md-nav__link"><span class="md-ellipsis"> Verifier </span></a>
- <a href="../../concepts/functions/" class="md-nav__link"><span class="md-ellipsis"> Functions </span></a>
- <a href="../../concepts/concurrency/" class="md-nav__link"><span class="md-ellipsis"> Concurrency </span></a>
- <a href="../../concepts/pinning/" class="md-nav__link"><span class="md-ellipsis"> Pinning </span></a>
- <a href="../../concepts/tail-calls/" class="md-nav__link"><span class="md-ellipsis"> Tail calls </span></a>
- <a href="../../concepts/loops/" class="md-nav__link"><span class="md-ellipsis"> Loops </span></a>
- <a href="../../concepts/timers/" class="md-nav__link"><span class="md-ellipsis"> Timers </span></a>
- <a href="../../concepts/resource-limit/" class="md-nav__link"><span class="md-ellipsis"> Resource Limit </span></a>
- <a href="../../concepts/af_xdp/" class="md-nav__link"><span class="md-ellipsis"> AF_XDP </span></a>
- <a href="../../concepts/kfuncs/" class="md-nav__link"><span class="md-ellipsis"> KFuncs </span></a>
- <a href="../../concepts/dynptrs/" class="md-nav__link"><span class="md-ellipsis"> Dynptrs </span></a>
- <a href="../../concepts/token/" class="md-nav__link"><span class="md-ellipsis"> Token </span></a>
- <a href="../../concepts/trampolines/" class="md-nav__link"><span class="md-ellipsis"> Trampolines </span></a>
- <a href="../../concepts/usdt/" class="md-nav__link"><span class="md-ellipsis"> USDT </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../program-type/" class="md-nav__link"><span class="md-ellipsis"> Program types </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Program types

<span class="md-ellipsis"> Network program types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Network program types

<a href="../../program-type/BPF_PROG_TYPE_SOCKET_FILTER/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SOCKET_FILTER </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SCHED_CLS/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SCHED_CLS </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SCHED_ACT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SCHED_ACT </span></a>

<a href="../../program-type/BPF_PROG_TYPE_XDP/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_XDP </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SOCK_OPS/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SOCK_OPS </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SK_SKB/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SK_SKB </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SK_MSG/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SK_MSG </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SK_LOOKUP/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SK_LOOKUP </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SK_REUSEPORT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SK_REUSEPORT </span></a>

<a href="../../program-type/BPF_PROG_TYPE_FLOW_DISSECTOR/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_FLOW_DISSECTOR </span></a>

<a href="../../program-type/BPF_PROG_TYPE_NETFILTER/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_NETFILTER </span></a>

<span class="md-ellipsis"> Light weight tunnel program types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Light weight tunnel program types

- <a href="../../program-type/BPF_PROG_TYPE_LWT_IN/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_LWT_IN </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_LWT_OUT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_LWT_OUT </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_LWT_XMIT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_LWT_XMIT </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_LWT_SEG6LOCAL/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_LWT_SEG6LOCAL </span></a>

<span class="md-ellipsis"> cGroup program types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> cGroup program types

- <a href="../../program-type/BPF_PROG_TYPE_CGROUP_SKB/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_CGROUP_SKB </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_CGROUP_SOCK/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_CGROUP_SOCK </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_CGROUP_DEVICE/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_CGROUP_DEVICE </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_CGROUP_SOCK_ADDR/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_CGROUP_SOCK_ADDR </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_CGROUP_SOCKOPT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_CGROUP_SOCKOPT </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_CGROUP_SYSCTL/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_CGROUP_SYSCTL </span></a>

<span class="md-ellipsis"> Tracing program types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Tracing program types

- <a href="../../program-type/BPF_PROG_TYPE_KPROBE/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_KPROBE </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_TRACEPOINT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_TRACEPOINT </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_PERF_EVENT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_PERF_EVENT </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_RAW_TRACEPOINT </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_TRACING/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_TRACING </span></a>

<a href="../../program-type/BPF_PROG_TYPE_LIRC_MODE2/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_LIRC_MODE2 </span></a>

<a href="../../program-type/BPF_PROG_TYPE_LSM/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_LSM </span></a>

<a href="../../program-type/BPF_PROG_TYPE_EXT/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_EXT </span></a>

<span class="md-ellipsis"> BPF_PROG_TYPE_STRUCT_OPS </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF_PROG_TYPE_STRUCT_OPS

- <a href="../../program-type/BPF_PROG_TYPE_STRUCT_OPS/" class="md-nav__link"><span class="md-ellipsis"> Program Type 'BPF_PROG_TYPE_STRUCT_OPS' </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_STRUCT_OPS/tcp_congestion_ops/" class="md-nav__link"><span class="md-ellipsis"> struct tcp_congestion_ops </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_STRUCT_OPS/hid_bpf_ops/" class="md-nav__link"><span class="md-ellipsis"> struct hid_bpf_ops </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_STRUCT_OPS/sched_ext_ops/" class="md-nav__link"><span class="md-ellipsis"> struct sched_ext_ops </span></a>
- <a href="../../program-type/BPF_PROG_TYPE_STRUCT_OPS/Qdisc_ops/" class="md-nav__link"><span class="md-ellipsis"> struct Qdisc_ops </span></a>

<a href="../../program-type/BPF_PROG_TYPE_SYSCALL/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TYPE_SYSCALL </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../map-type/" class="md-nav__link"><span class="md-ellipsis"> Map types </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Map types

<span class="md-ellipsis"> Generic map types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Generic map types

- <a href="../../map-type/BPF_MAP_TYPE_HASH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_HASH </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_ARRAY </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_PERCPU_HASH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_PERCPU_HASH </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_PERCPU_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_PERCPU_ARRAY </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_QUEUE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_QUEUE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_STACK/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_STACK </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_LRU_HASH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_LRU_HASH </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_LRU_PERCPU_HASH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_LRU_PERCPU_HASH </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_LPM_TRIE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_LPM_TRIE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_BLOOM_FILTER/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_BLOOM_FILTER </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_ARENA/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_ARENA </span></a>

<span class="md-ellipsis"> Map in map </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Map in map

- <a href="../../map-type/BPF_MAP_TYPE_ARRAY_OF_MAPS/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_ARRAY_OF_MAPS </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_HASH_OF_MAPS/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_HASH_OF_MAPS </span></a>

<span class="md-ellipsis"> Streaming </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Streaming

- <a href="../../map-type/BPF_MAP_TYPE_PERF_EVENT_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_PERF_EVENT_ARRAY </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_RINGBUF/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_RINGBUF </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_USER_RINGBUF/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_USER_RINGBUF </span></a>

<span class="md-ellipsis"> Packet redirection </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Packet redirection

- <a href="../../map-type/BPF_MAP_TYPE_DEVMAP/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_DEVMAP </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_SOCKMAP/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_SOCKMAP </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_CPUMAP/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_CPUMAP </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_XSKMAP/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_XSKMAP </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_SOCKHASH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_SOCKHASH </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_DEVMAP_HASH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_DEVMAP_HASH </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_REUSEPORT_SOCKARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_REUSEPORT_SOCKARRAY </span></a>

<span class="md-ellipsis"> Flow redirection </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Flow redirection

- <a href="../../map-type/BPF_MAP_TYPE_PROG_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_PROG_ARRAY </span></a>

<span class="md-ellipsis"> Object attached storage </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Object attached storage

- <a href="../../map-type/BPF_MAP_TYPE_CGROUP_STORAGE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_CGROUP_STORAGE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_SK_STORAGE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_SK_STORAGE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_INODE_STORAGE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_INODE_STORAGE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_TASK_STORAGE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_TASK_STORAGE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_CGRP_STORAGE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_CGRP_STORAGE </span></a>

<span class="md-ellipsis"> Misc </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Misc

- <a href="../../map-type/BPF_MAP_TYPE_CGROUP_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_CGROUP_ARRAY </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_STACK_TRACE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_STACK_TRACE </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_STRUCT_OPS/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_STRUCT_OPS </span></a>
- <a href="../../map-type/BPF_MAP_TYPE_INSN_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_TYPE_INSN_ARRAY </span></a>

<div class="md-nav__link md-nav__container">

<a href="../" class="md-nav__link"><span class="md-ellipsis"> Helper functions </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Helper functions

<span class="md-ellipsis"> Map helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Map helpers

<span class="md-ellipsis"> Generic map helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Generic map helpers

- <a href="../bpf_map_lookup_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_elem </span></a>
- <a href="../bpf_map_update_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_update_elem </span></a>
- <a href="../bpf_map_delete_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_delete_elem </span></a>
- <a href="../bpf_for_each_map_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_for_each_map_elem </span></a>
- <a href="../bpf_map_lookup_percpu_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_percpu_elem </span></a>
- <a href="../bpf_spin_lock/" class="md-nav__link"><span class="md-ellipsis"> bpf_spin_lock </span></a>
- <a href="../bpf_spin_unlock/" class="md-nav__link"><span class="md-ellipsis"> bpf_spin_unlock </span></a>

<span class="md-ellipsis"> Perf event array helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Perf event array helpers

- <a href="../bpf_perf_event_read/" class="md-nav__link"><span class="md-ellipsis"> bpf_perf_event_read </span></a>
- <a href="../bpf_perf_event_output/" class="md-nav__link"><span class="md-ellipsis"> bpf_perf_event_output </span></a>
- <a href="../bpf_perf_event_read_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_perf_event_read_value </span></a>
- <a href="../bpf_skb_output/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_output </span></a>
- <a href="../bpf_xdp_output/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_output </span></a>

<span class="md-ellipsis"> Tail call helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Tail call helpers

- <a href="../bpf_tail_call/" class="md-nav__link"><span class="md-ellipsis"> bpf_tail_call </span></a>

<span class="md-ellipsis"> Timer helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Timer helpers

- <a href="../bpf_timer_init/" class="md-nav__link"><span class="md-ellipsis"> bpf_timer_init </span></a>
- <a href="../bpf_timer_set_callback/" class="md-nav__link"><span class="md-ellipsis"> bpf_timer_set_callback </span></a>
- <a href="../bpf_timer_start/" class="md-nav__link"><span class="md-ellipsis"> bpf_timer_start </span></a>
- <a href="../bpf_timer_cancel/" class="md-nav__link"><span class="md-ellipsis"> bpf_timer_cancel </span></a>

<span class="md-ellipsis"> Queue and stack helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Queue and stack helpers

- <a href="../bpf_map_push_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_push_elem </span></a>
- <a href="../bpf_map_pop_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_pop_elem </span></a>
- <a href="../bpf_map_peek_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_peek_elem </span></a>

<span class="md-ellipsis"> Ring buffer helper </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Ring buffer helper

<span class="md-ellipsis"> bpf_ringbuf_output </span> <span class="md-nav__icon md-icon"></span> <a href="./" class="md-nav__link md-nav__link--active"><span class="md-ellipsis"> bpf_ringbuf_output </span></a>

<span class="md-nav__icon md-icon"></span> Table of contents

- <a href="#definition" class="md-nav__link"><span class="md-ellipsis"> Definition </span></a>
  - <a href="#returns" class="md-nav__link"><span class="md-ellipsis"> Returns </span></a>
- <a href="#usage" class="md-nav__link"><span class="md-ellipsis"> Usage </span></a>
  - <a href="#program-types" class="md-nav__link"><span class="md-ellipsis"> Program types </span></a>
  - <a href="#example" class="md-nav__link"><span class="md-ellipsis"> Example </span></a>

<a href="../bpf_ringbuf_reserve/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_reserve </span></a>

<a href="../bpf_ringbuf_submit/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_submit </span></a>

<a href="../bpf_ringbuf_discard/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_discard </span></a>

<a href="../bpf_ringbuf_query/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_query </span></a>

<a href="../bpf_ringbuf_reserve_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_reserve_dynptr </span></a>

<a href="../bpf_ringbuf_submit_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_submit_dynptr </span></a>

<a href="../bpf_ringbuf_discard_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_ringbuf_discard_dynptr </span></a>

<span class="md-ellipsis"> Socket map helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket map helpers

- <a href="../bpf_sock_map_update/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_map_update </span></a>

<span class="md-ellipsis"> Socket hash helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket hash helpers

- <a href="../bpf_sock_hash_update/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_hash_update </span></a>

<span class="md-ellipsis"> Task storage helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Task storage helpers

- <a href="../bpf_task_storage_get/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_storage_get </span></a>
- <a href="../bpf_task_storage_delete/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_storage_delete </span></a>

<span class="md-ellipsis"> Inode storage helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Inode storage helpers

- <a href="../bpf_inode_storage_get/" class="md-nav__link"><span class="md-ellipsis"> bpf_inode_storage_get </span></a>
- <a href="../bpf_inode_storage_delete/" class="md-nav__link"><span class="md-ellipsis"> bpf_inode_storage_delete </span></a>

<span class="md-ellipsis"> Socket storage helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket storage helpers

- <a href="../bpf_sk_storage_get/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_storage_get </span></a>
- <a href="../bpf_sk_storage_delete/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_storage_delete </span></a>

<span class="md-ellipsis"> Local cGroup storage helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Local cGroup storage helpers

- <a href="../bpf_get_local_storage/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_local_storage </span></a>

<span class="md-ellipsis"> Global cGroup storage helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Global cGroup storage helpers

- <a href="../bpf_cgrp_storage_get/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgrp_storage_get </span></a>
- <a href="../bpf_cgrp_storage_delete/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgrp_storage_delete </span></a>

<span class="md-ellipsis"> User ring buffer </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> User ring buffer

- <a href="../bpf_user_ringbuf_drain/" class="md-nav__link"><span class="md-ellipsis"> bpf_user_ringbuf_drain </span></a>

<span class="md-ellipsis"> Probe and trace helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Probe and trace helpers

<a href="../bpf_get_attach_cookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_attach_cookie </span></a>

<span class="md-ellipsis"> Memory helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Memory helpers

- <a href="../bpf_probe_read/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read </span></a>
- <a href="../bpf_probe_write_user/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_write_user </span></a>
- <a href="../bpf_probe_read_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_str </span></a>
- <a href="../bpf_get_stack/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_stack </span></a>
- <a href="../bpf_probe_read_user/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_user </span></a>
- <a href="../bpf_probe_read_kernel/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_kernel </span></a>
- <a href="../bpf_probe_read_user_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_user_str </span></a>
- <a href="../bpf_probe_read_kernel_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_kernel_str </span></a>
- <a href="../bpf_copy_from_user/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user </span></a>
- <a href="../bpf_copy_from_user_task/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_task </span></a>
- <a href="../bpf_copy_from_user_task/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_task </span></a>
- <a href="../bpf_find_vma/" class="md-nav__link"><span class="md-ellipsis"> bpf_find_vma </span></a>

<span class="md-ellipsis"> Process influencing helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Process influencing helpers

- <a href="../bpf_override_return/" class="md-nav__link"><span class="md-ellipsis"> bpf_override_return </span></a>
- <a href="../bpf_get_retval/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_retval </span></a>
- <a href="../bpf_set_retval/" class="md-nav__link"><span class="md-ellipsis"> bpf_set_retval </span></a>
- <a href="../bpf_send_signal/" class="md-nav__link"><span class="md-ellipsis"> bpf_send_signal </span></a>
- <a href="../bpf_send_signal_thread/" class="md-nav__link"><span class="md-ellipsis"> bpf_send_signal_thread </span></a>

<span class="md-ellipsis"> Tracing helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Tracing helpers

- <a href="../bpf_get_func_ip/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_func_ip </span></a>
- <a href="../bpf_get_func_arg/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_func_arg </span></a>
- <a href="../bpf_get_func_ret/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_func_ret </span></a>
- <a href="../bpf_get_func_arg_cnt/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_func_arg_cnt </span></a>
- <a href="../bpf_sock_from_file/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_from_file </span></a>

<span class="md-ellipsis"> Perf event program helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Perf event program helpers

- <a href="../bpf_perf_prog_read_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_perf_prog_read_value </span></a>

<span class="md-ellipsis"> Information helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Information helpers

<span class="md-ellipsis"> Time helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Time helpers

- <a href="../bpf_ktime_get_ns/" class="md-nav__link"><span class="md-ellipsis"> bpf_ktime_get_ns </span></a>
- <a href="../bpf_jiffies64/" class="md-nav__link"><span class="md-ellipsis"> bpf_jiffies64 </span></a>
- <a href="../bpf_ktime_get_boot_ns/" class="md-nav__link"><span class="md-ellipsis"> bpf_ktime_get_boot_ns </span></a>
- <a href="../bpf_ktime_get_coarse_ns/" class="md-nav__link"><span class="md-ellipsis"> bpf_ktime_get_coarse_ns </span></a>
- <a href="../bpf_ktime_get_tai_ns/" class="md-nav__link"><span class="md-ellipsis"> bpf_ktime_get_tai_ns </span></a>

<span class="md-ellipsis"> Process info helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Process info helpers

- <a href="../bpf_get_current_pid_tgid/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_pid_tgid </span></a>
- <a href="../bpf_get_current_uid_gid/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_uid_gid </span></a>
- <a href="../bpf_get_current_comm/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_comm </span></a>
- <a href="../bpf_get_cgroup_classid/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_cgroup_classid </span></a>
- <a href="../bpf_get_ns_current_pid_tgid/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_ns_current_pid_tgid </span></a>
- <a href="../bpf_get_current_task/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_task </span></a>
- <a href="../bpf_get_stackid/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_stackid </span></a>
- <a href="../bpf_current_task_under_cgroup/" class="md-nav__link"><span class="md-ellipsis"> bpf_current_task_under_cgroup </span></a>
- <a href="../bpf_get_current_cgroup_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_cgroup_id </span></a>
- <a href="../bpf_get_current_ancestor_cgroup_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_ancestor_cgroup_id </span></a>
- <a href="../bpf_get_task_stack/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_task_stack </span></a>
- <a href="../bpf_get_current_task_btf/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_current_task_btf </span></a>
- <a href="../bpf_task_pt_regs/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_pt_regs </span></a>

<span class="md-ellipsis"> CPU info helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CPU info helpers

- <a href="../bpf_get_smp_processor_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_smp_processor_id </span></a>
- <a href="../bpf_get_numa_node_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_numa_node_id </span></a>
- <a href="../bpf_read_branch_records/" class="md-nav__link"><span class="md-ellipsis"> bpf_read_branch_records </span></a>
- <a href="../bpf_get_branch_snapshot/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_branch_snapshot </span></a>
- <a href="../bpf_per_cpu_ptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_per_cpu_ptr </span></a>
- <a href="../bpf_this_cpu_ptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_this_cpu_ptr </span></a>

<span class="md-ellipsis"> Print helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Print helpers

<a href="../bpf_trace_printk/" class="md-nav__link"><span class="md-ellipsis"> bpf_trace_printk </span></a>

<a href="../bpf_snprintf/" class="md-nav__link"><span class="md-ellipsis"> bpf_snprintf </span></a>

<a href="../bpf_snprintf_btf/" class="md-nav__link"><span class="md-ellipsis"> bpf_snprintf_btf </span></a>

<a href="../bpf_trace_vprintk/" class="md-nav__link"><span class="md-ellipsis"> bpf_trace_vprintk </span></a>

<span class="md-ellipsis"> Iterator print helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Iterator print helpers

- <a href="../bpf_seq_printf/" class="md-nav__link"><span class="md-ellipsis"> bpf_seq_printf </span></a>
- <a href="../bpf_seq_write/" class="md-nav__link"><span class="md-ellipsis"> bpf_seq_write </span></a>
- <a href="../bpf_seq_printf_btf/" class="md-nav__link"><span class="md-ellipsis"> bpf_seq_printf_btf </span></a>

<span class="md-ellipsis"> Network helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Network helpers

<a href="../bpf_get_netns_cookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_netns_cookie </span></a>

<a href="../bpf_check_mtu/" class="md-nav__link"><span class="md-ellipsis"> bpf_check_mtu </span></a>

<a href="../bpf_get_route_realm/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_route_realm </span></a>

<a href="../bpf_fib_lookup/" class="md-nav__link"><span class="md-ellipsis"> bpf_fib_lookup </span></a>

<span class="md-ellipsis"> Socket buffer helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket buffer helpers

- <a href="../bpf_skb_store_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_store_bytes </span></a>
- <a href="../bpf_skb_load_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_load_bytes </span></a>
- <a href="../bpf_skb_vlan_push/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_vlan_push </span></a>
- <a href="../bpf_skb_vlan_pop/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_vlan_pop </span></a>
- <a href="../bpf_skb_get_tunnel_key/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_get_tunnel_key </span></a>
- <a href="../bpf_skb_set_tunnel_key/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_set_tunnel_key </span></a>
- <a href="../bpf_skb_get_tunnel_opt/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_get_tunnel_opt </span></a>
- <a href="../bpf_skb_set_tunnel_opt/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_set_tunnel_opt </span></a>
- <a href="../bpf_skb_change_proto/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_change_proto </span></a>
- <a href="../bpf_skb_change_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_change_type </span></a>
- <a href="../bpf_skb_under_cgroup/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_under_cgroup </span></a>
- <a href="../bpf_skb_change_tail/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_change_tail </span></a>
- <a href="../bpf_skb_pull_data/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_pull_data </span></a>
- <a href="../bpf_skb_adjust_room/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_adjust_room </span></a>
- <a href="../bpf_skb_change_head/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_change_head </span></a>
- <a href="../bpf_skb_get_xfrm_state/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_get_xfrm_state </span></a>
- <a href="../bpf_skb_load_bytes_relative/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_load_bytes_relative </span></a>
- <a href="../bpf_skb_cgroup_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_cgroup_id </span></a>
- <a href="../bpf_skb_ancestor_cgroup_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_ancestor_cgroup_id </span></a>
- <a href="../bpf_skb_ecn_set_ce/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_ecn_set_ce </span></a>
- <a href="../bpf_skb_cgroup_classid/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_cgroup_classid </span></a>
- <a href="../bpf_skb_set_tstamp/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_set_tstamp </span></a>
- <a href="../bpf_set_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_set_hash </span></a>
- <a href="../bpf_get_hash_recalc/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_hash_recalc </span></a>
- <a href="../bpf_set_hash_invalid/" class="md-nav__link"><span class="md-ellipsis"> bpf_set_hash_invalid </span></a>

<span class="md-ellipsis"> Checksum helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Checksum helpers

- <a href="../bpf_l3_csum_replace/" class="md-nav__link"><span class="md-ellipsis"> bpf_l3_csum_replace </span></a>
- <a href="../bpf_l4_csum_replace/" class="md-nav__link"><span class="md-ellipsis"> bpf_l4_csum_replace </span></a>
- <a href="../bpf_csum_diff/" class="md-nav__link"><span class="md-ellipsis"> bpf_csum_diff </span></a>
- <a href="../bpf_csum_update/" class="md-nav__link"><span class="md-ellipsis"> bpf_csum_update </span></a>
- <a href="../bpf_csum_level/" class="md-nav__link"><span class="md-ellipsis"> bpf_csum_level </span></a>

<span class="md-ellipsis"> Redirect helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Redirect helpers

- <a href="../bpf_clone_redirect/" class="md-nav__link"><span class="md-ellipsis"> bpf_clone_redirect </span></a>
- <a href="../bpf_redirect/" class="md-nav__link"><span class="md-ellipsis"> bpf_redirect </span></a>
- <a href="../bpf_redirect_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_redirect_map </span></a>
- <a href="../bpf_sk_redirect_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_redirect_map </span></a>
- <a href="../bpf_msg_redirect_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_redirect_map </span></a>
- <a href="../bpf_redirect_peer/" class="md-nav__link"><span class="md-ellipsis"> bpf_redirect_peer </span></a>
- <a href="../bpf_sk_redirect_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_redirect_hash </span></a>
- <a href="../bpf_msg_redirect_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_redirect_hash </span></a>
- <a href="../bpf_redirect_neigh/" class="md-nav__link"><span class="md-ellipsis"> bpf_redirect_neigh </span></a>
- <a href="../bpf_sk_select_reuseport/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_select_reuseport </span></a>
- <a href="../bpf_sk_assign/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_assign </span></a>

<span class="md-ellipsis"> XDP helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> XDP helpers

- <a href="../bpf_xdp_adjust_head/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_adjust_head </span></a>
- <a href="../bpf_xdp_adjust_tail/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_adjust_tail </span></a>
- <a href="../bpf_xdp_adjust_meta/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_adjust_meta </span></a>
- <a href="../bpf_xdp_get_buff_len/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_get_buff_len </span></a>
- <a href="../bpf_xdp_load_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_load_bytes </span></a>
- <a href="../bpf_xdp_store_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_store_bytes </span></a>

<span class="md-ellipsis"> Socket message helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket message helpers

- <a href="../bpf_msg_apply_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_apply_bytes </span></a>
- <a href="../bpf_msg_cork_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_cork_bytes </span></a>
- <a href="../bpf_msg_pull_data/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_pull_data </span></a>
- <a href="../bpf_msg_push_data/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_push_data </span></a>
- <a href="../bpf_msg_pop_data/" class="md-nav__link"><span class="md-ellipsis"> bpf_msg_pop_data </span></a>

<span class="md-ellipsis"> LWT helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> LWT helpers

- <a href="../bpf_lwt_push_encap/" class="md-nav__link"><span class="md-ellipsis"> bpf_lwt_push_encap </span></a>
- <a href="../bpf_lwt_seg6_store_bytes/" class="md-nav__link"><span class="md-ellipsis"> bpf_lwt_seg6_store_bytes </span></a>
- <a href="../bpf_lwt_seg6_adjust_srh/" class="md-nav__link"><span class="md-ellipsis"> bpf_lwt_seg6_adjust_srh </span></a>
- <a href="../bpf_lwt_seg6_action/" class="md-nav__link"><span class="md-ellipsis"> bpf_lwt_seg6_action </span></a>

<span class="md-ellipsis"> SYN Cookie helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> SYN Cookie helpers

- <a href="../bpf_tcp_check_syncookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_check_syncookie </span></a>
- <a href="../bpf_tcp_gen_syncookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_gen_syncookie </span></a>
- <a href="../bpf_tcp_raw_gen_syncookie_ipv4/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_raw_gen_syncookie_ipv4 </span></a>
- <a href="../bpf_tcp_raw_gen_syncookie_ipv6/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_raw_gen_syncookie_ipv6 </span></a>
- <a href="../bpf_tcp_raw_check_syncookie_ipv4/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_raw_check_syncookie_ipv4 </span></a>
- <a href="../bpf_tcp_raw_check_syncookie_ipv6/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_raw_check_syncookie_ipv6 </span></a>

<span class="md-ellipsis"> Socket helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket helpers

- <a href="../bpf_sk_lookup_tcp/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_lookup_tcp </span></a>
- <a href="../bpf_sk_lookup_udp/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_lookup_udp </span></a>
- <a href="../bpf_sk_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_release </span></a>
- <a href="../bpf_sk_fullsock/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_fullsock </span></a>
- <a href="../bpf_sk_cgroup_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_cgroup_id </span></a>
- <a href="../bpf_sk_ancestor_cgroup_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_ancestor_cgroup_id </span></a>
- <a href="../bpf_get_socket_cookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_socket_cookie </span></a>
- <a href="../bpf_get_socket_uid/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_socket_uid </span></a>
- <a href="../bpf_setsockopt/" class="md-nav__link"><span class="md-ellipsis"> bpf_setsockopt </span></a>
- <a href="../bpf_getsockopt/" class="md-nav__link"><span class="md-ellipsis"> bpf_getsockopt </span></a>
- <a href="../bpf_sock_ops_cb_flags_set/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_ops_cb_flags_set </span></a>
- <a href="../bpf_tcp_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_sock </span></a>
- <a href="../bpf_get_listener_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_listener_sock </span></a>
- <a href="../bpf_tcp_send_ack/" class="md-nav__link"><span class="md-ellipsis"> bpf_tcp_send_ack </span></a>
- <a href="../bpf_skc_lookup_tcp/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_lookup_tcp </span></a>
- <a href="../bpf_skc_to_tcp6_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_tcp6_sock </span></a>
- <a href="../bpf_skc_to_tcp_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_tcp_sock </span></a>
- <a href="../bpf_skc_to_tcp_timewait_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_tcp_timewait_sock </span></a>
- <a href="../bpf_skc_to_tcp_request_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_tcp_request_sock </span></a>
- <a href="../bpf_skc_to_udp6_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_udp6_sock </span></a>
- <a href="../bpf_skc_to_mptcp_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_mptcp_sock </span></a>
- <a href="../bpf_skc_to_unix_sock/" class="md-nav__link"><span class="md-ellipsis"> bpf_skc_to_unix_sock </span></a>
- <a href="../bpf_bind/" class="md-nav__link"><span class="md-ellipsis"> bpf_bind </span></a>

<span class="md-ellipsis"> Socket ops helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket ops helpers

- <a href="../bpf_load_hdr_opt/" class="md-nav__link"><span class="md-ellipsis"> bpf_load_hdr_opt </span></a>
- <a href="../bpf_store_hdr_opt/" class="md-nav__link"><span class="md-ellipsis"> bpf_store_hdr_opt </span></a>
- <a href="../bpf_reserve_hdr_opt/" class="md-nav__link"><span class="md-ellipsis"> bpf_reserve_hdr_opt </span></a>

<span class="md-ellipsis"> Infrared related helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Infrared related helpers

- <a href="../bpf_rc_repeat/" class="md-nav__link"><span class="md-ellipsis"> bpf_rc_repeat </span></a>
- <a href="../bpf_rc_keydown/" class="md-nav__link"><span class="md-ellipsis"> bpf_rc_keydown </span></a>
- <a href="../bpf_rc_pointer_rel/" class="md-nav__link"><span class="md-ellipsis"> bpf_rc_pointer_rel </span></a>

<span class="md-ellipsis"> Syscall helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Syscall helpers

- <a href="../bpf_sys_bpf/" class="md-nav__link"><span class="md-ellipsis"> bpf_sys_bpf </span></a>
- <a href="../bpf_btf_find_by_name_kind/" class="md-nav__link"><span class="md-ellipsis"> bpf_btf_find_by_name_kind </span></a>
- <a href="../bpf_sys_close/" class="md-nav__link"><span class="md-ellipsis"> bpf_sys_close </span></a>
- <a href="../bpf_kallsyms_lookup_name/" class="md-nav__link"><span class="md-ellipsis"> bpf_kallsyms_lookup_name </span></a>

<span class="md-ellipsis"> LSM helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> LSM helpers

- <a href="../bpf_bprm_opts_set/" class="md-nav__link"><span class="md-ellipsis"> bpf_bprm_opts_set </span></a>
- <a href="../bpf_ima_inode_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_ima_inode_hash </span></a>
- <a href="../bpf_ima_file_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_ima_file_hash </span></a>

<span class="md-ellipsis"> Sysctl helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Sysctl helpers

- <a href="../bpf_sysctl_get_name/" class="md-nav__link"><span class="md-ellipsis"> bpf_sysctl_get_name </span></a>
- <a href="../bpf_sysctl_get_current_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_sysctl_get_current_value </span></a>
- <a href="../bpf_sysctl_get_new_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_sysctl_get_new_value </span></a>
- <a href="../bpf_sysctl_set_new_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_sysctl_set_new_value </span></a>

<span class="md-ellipsis"> Dynptr </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Dynptr

- <a href="../bpf_dynptr_from_mem/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_from_mem </span></a>
- <a href="../bpf_dynptr_read/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_read </span></a>
- <a href="../bpf_dynptr_write/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_write </span></a>
- <a href="../bpf_dynptr_data/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_data </span></a>

<span class="md-ellipsis"> Loop helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Loop helpers

- <a href="../bpf_loop/" class="md-nav__link"><span class="md-ellipsis"> bpf_loop </span></a>

<span class="md-ellipsis"> Utility helpers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Utility helpers

- <a href="../bpf_get_prandom_u32/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_prandom_u32 </span></a>
- <a href="../bpf_strtol/" class="md-nav__link"><span class="md-ellipsis"> bpf_strtol </span></a>
- <a href="../bpf_strtoul/" class="md-nav__link"><span class="md-ellipsis"> bpf_strtoul </span></a>
- <a href="../bpf_strncmp/" class="md-nav__link"><span class="md-ellipsis"> bpf_strncmp </span></a>
- <a href="../bpf_d_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_d_path </span></a>

<span class="md-ellipsis"> Misc </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Misc

- <a href="../bpf_kptr_xchg/" class="md-nav__link"><span class="md-ellipsis"> bpf_kptr_xchg </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../syscall/" class="md-nav__link"><span class="md-ellipsis"> Syscall commands </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Syscall commands

<span class="md-ellipsis"> Object creation commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Object creation commands

- <a href="../../syscall/BPF_MAP_CREATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_CREATE </span></a>
- <a href="../../syscall/BPF_PROG_LOAD/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_LOAD </span></a>
- <a href="../../syscall/BPF_BTF_LOAD/" class="md-nav__link"><span class="md-ellipsis"> BPF_BTF_LOAD </span></a>
- <a href="../../syscall/BPF_LINK_CREATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_LINK_CREATE </span></a>
- <a href="../../syscall/BPF_ITER_CREATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_ITER_CREATE </span></a>
- <a href="../../syscall/BPF_RAW_TRACEPOINT_OPEN/" class="md-nav__link"><span class="md-ellipsis"> BPF_RAW_TRACEPOINT_OPEN </span></a>

<span class="md-ellipsis"> Map commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Map commands

- <a href="../../syscall/BPF_MAP_CREATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_CREATE </span></a>
- <a href="../../syscall/BPF_MAP_LOOKUP_ELEM/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_LOOKUP_ELEM </span></a>
- <a href="../../syscall/BPF_MAP_UPDATE_ELEM/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_UPDATE_ELEM </span></a>
- <a href="../../syscall/BPF_MAP_DELETE_ELEM/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_DELETE_ELEM </span></a>
- <a href="../../syscall/BPF_MAP_GET_NEXT_KEY/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_GET_NEXT_KEY </span></a>
- <a href="../../syscall/BPF_MAP_LOOKUP_BATCH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_LOOKUP_BATCH </span></a>
- <a href="../../syscall/BPF_MAP_LOOKUP_AND_DELETE_BATCH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_LOOKUP_AND_DELETE_BATCH </span></a>
- <a href="../../syscall/BPF_MAP_UPDATE_BATCH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_UPDATE_BATCH </span></a>
- <a href="../../syscall/BPF_MAP_DELETE_BATCH/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_DELETE_BATCH </span></a>
- <a href="../../syscall/BPF_MAP_LOOKUP_AND_DELETE_ELEM/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_LOOKUP_AND_DELETE_ELEM </span></a>
- <a href="../../syscall/BPF_MAP_FREEZE/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_FREEZE </span></a>

<span class="md-ellipsis"> Pin commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Pin commands

- <a href="../../syscall/BPF_OBJ_PIN/" class="md-nav__link"><span class="md-ellipsis"> BPF_OBJ_PIN </span></a>
- <a href="../../syscall/BPF_OBJ_GET/" class="md-nav__link"><span class="md-ellipsis"> BPF_OBJ_GET </span></a>

<span class="md-ellipsis"> Program commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Program commands

- <a href="../../syscall/BPF_PROG_LOAD/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_LOAD </span></a>
- <a href="../../syscall/BPF_PROG_ATTACH/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_ATTACH </span></a>
- <a href="../../syscall/BPF_PROG_DETACH/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_DETACH </span></a>
- <a href="../../syscall/BPF_PROG_TEST_RUN/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TEST_RUN </span></a>
- <a href="../../syscall/BPF_PROG_TEST_RUN/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_TEST_RUN </span></a>
- <a href="../../syscall/BPF_PROG_BIND_MAP/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_BIND_MAP </span></a>

<span class="md-ellipsis"> Object discovery commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Object discovery commands

- <a href="../../syscall/BPF_PROG_GET_NEXT_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_GET_NEXT_ID </span></a>
- <a href="../../syscall/BPF_MAP_GET_NEXT_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_GET_NEXT_ID </span></a>
- <a href="../../syscall/BPF_PROG_GET_FD_BY_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_GET_FD_BY_ID </span></a>
- <a href="../../syscall/BPF_MAP_GET_FD_BY_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_MAP_GET_FD_BY_ID </span></a>
- <a href="../../syscall/BPF_OBJ_GET_INFO_BY_FD/" class="md-nav__link"><span class="md-ellipsis"> BPF_OBJ_GET_INFO_BY_FD </span></a>
- <a href="../../syscall/BPF_PROG_QUERY/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG_QUERY </span></a>
- <a href="../../syscall/BPF_BTF_GET_FD_BY_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_BTF_GET_FD_BY_ID </span></a>
- <a href="../../syscall/BPF_TASK_FD_QUERY/" class="md-nav__link"><span class="md-ellipsis"> BPF_TASK_FD_QUERY </span></a>
- <a href="../../syscall/BPF_BTF_GET_NEXT_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_BTF_GET_NEXT_ID </span></a>
- <a href="../../syscall/BPF_LINK_GET_FD_BY_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_LINK_GET_FD_BY_ID </span></a>
- <a href="../../syscall/BPF_LINK_GET_NEXT_ID/" class="md-nav__link"><span class="md-ellipsis"> BPF_LINK_GET_NEXT_ID </span></a>

<span class="md-ellipsis"> Link commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Link commands

- <a href="../../syscall/BPF_LINK_CREATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_LINK_CREATE </span></a>
- <a href="../../syscall/BPF_LINK_UPDATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_LINK_UPDATE </span></a>
- <a href="../../syscall/BPF_LINK_DETACH/" class="md-nav__link"><span class="md-ellipsis"> BPF_LINK_DETACH </span></a>

<span class="md-ellipsis"> Statistics commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Statistics commands

- <a href="../../syscall/BPF_ENABLE_STATS/" class="md-nav__link"><span class="md-ellipsis"> BPF_ENABLE_STATS </span></a>

<span class="md-ellipsis"> Security commands </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Security commands

- <a href="../../syscall/BPF_TOKEN_CREATE/" class="md-nav__link"><span class="md-ellipsis"> BPF_TOKEN_CREATE </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../kfuncs/" class="md-nav__link"><span class="md-ellipsis"> KFuncs </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> KFuncs

<span class="md-ellipsis"> cGroup resource stats KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> cGroup resource stats KFuncs

- <a href="../../kfuncs/cgroup_rstat_updated/" class="md-nav__link"><span class="md-ellipsis"> cgroup_rstat_updated </span></a>
- <a href="../../kfuncs/cgroup_rstat_flush/" class="md-nav__link"><span class="md-ellipsis"> cgroup_rstat_flush </span></a>
- <a href="../../kfuncs/css_rstat_updated/" class="md-nav__link"><span class="md-ellipsis"> css_rstat_updated </span></a>
- <a href="../../kfuncs/css_rstat_flush/" class="md-nav__link"><span class="md-ellipsis"> css_rstat_flush </span></a>

<span class="md-ellipsis"> Key signature verification KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Key signature verification KFuncs

- <a href="../../kfuncs/bpf_lookup_user_key/" class="md-nav__link"><span class="md-ellipsis"> bpf_lookup_user_key </span></a>
- <a href="../../kfuncs/bpf_lookup_system_key/" class="md-nav__link"><span class="md-ellipsis"> bpf_lookup_system_key </span></a>
- <a href="../../kfuncs/bpf_key_put/" class="md-nav__link"><span class="md-ellipsis"> bpf_key_put </span></a>
- <a href="../../kfuncs/bpf_verify_pkcs7_signature/" class="md-nav__link"><span class="md-ellipsis"> bpf_verify_pkcs7_signature </span></a>

<span class="md-ellipsis"> File related kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> File related kfuncs

- <a href="../../kfuncs/bpf_get_file_xattr/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_file_xattr </span></a>
- <a href="../../kfuncs/bpf_get_task_exe_file/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_task_exe_file </span></a>
- <a href="../../kfuncs/bpf_put_file/" class="md-nav__link"><span class="md-ellipsis"> bpf_put_file </span></a>
- <a href="../../kfuncs/bpf_path_d_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_path_d_path </span></a>
- <a href="../../kfuncs/bpf_get_dentry_xattr/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_dentry_xattr </span></a>
- <a href="../../kfuncs/bpf_remove_dentry_xattr/" class="md-nav__link"><span class="md-ellipsis"> bpf_remove_dentry_xattr </span></a>
- <a href="../../kfuncs/bpf_set_dentry_xattr/" class="md-nav__link"><span class="md-ellipsis"> bpf_set_dentry_xattr </span></a>

<span class="md-ellipsis"> CPU mask KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CPU mask KFuncs

- <a href="../../kfuncs/bpf_cpumask_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_create </span></a>
- <a href="../../kfuncs/bpf_cpumask_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_release </span></a>
- <a href="../../kfuncs/bpf_cpumask_acquire/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_acquire </span></a>
- <a href="../../kfuncs/bpf_cpumask_first/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_first </span></a>
- <a href="../../kfuncs/bpf_cpumask_first_zero/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_first_zero </span></a>
- <a href="../../kfuncs/bpf_cpumask_first_and/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_first_and </span></a>
- <a href="../../kfuncs/bpf_cpumask_set_cpu/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_set_cpu </span></a>
- <a href="../../kfuncs/bpf_cpumask_clear_cpu/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_clear_cpu </span></a>
- <a href="../../kfuncs/bpf_cpumask_test_cpu/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_test_cpu </span></a>
- <a href="../../kfuncs/bpf_cpumask_test_and_set_cpu/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_test_and_set_cpu </span></a>
- <a href="../../kfuncs/bpf_cpumask_test_and_clear_cpu/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_test_and_clear_cpu </span></a>
- <a href="../../kfuncs/bpf_cpumask_setall/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_setall </span></a>
- <a href="../../kfuncs/bpf_cpumask_clear/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_clear </span></a>
- <a href="../../kfuncs/bpf_cpumask_and/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_and </span></a>
- <a href="../../kfuncs/bpf_cpumask_or/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_or </span></a>
- <a href="../../kfuncs/bpf_cpumask_xor/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_xor </span></a>
- <a href="../../kfuncs/bpf_cpumask_equal/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_equal </span></a>
- <a href="../../kfuncs/bpf_cpumask_intersects/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_intersects </span></a>
- <a href="../../kfuncs/bpf_cpumask_subset/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_subset </span></a>
- <a href="../../kfuncs/bpf_cpumask_empty/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_empty </span></a>
- <a href="../../kfuncs/bpf_cpumask_full/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_full </span></a>
- <a href="../../kfuncs/bpf_cpumask_copy/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_copy </span></a>
- <a href="../../kfuncs/bpf_cpumask_any_distribute/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_any_distribute </span></a>
- <a href="../../kfuncs/bpf_cpumask_any_and_distribute/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_any_and_distribute </span></a>
- <a href="../../kfuncs/bpf_cpumask_weight/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_weight </span></a>
- <a href="../../kfuncs/bpf_cpumask_populate/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpumask_populate </span></a>

<span class="md-ellipsis"> Generic KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Generic KFuncs

- <a href="../../kfuncs/crash_kexec/" class="md-nav__link"><span class="md-ellipsis"> crash_kexec </span></a>
- <a href="../../kfuncs/bpf_throw/" class="md-nav__link"><span class="md-ellipsis"> bpf_throw </span></a>

<span class="md-ellipsis"> Object allocation KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Object allocation KFuncs

- <a href="../../kfuncs/bpf_obj_new_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_new_impl </span></a>
- <a href="../../kfuncs/bpf_percpu_obj_new_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_percpu_obj_new_impl </span></a>
- <a href="../../kfuncs/bpf_obj_drop_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_drop_impl </span></a>
- <a href="../../kfuncs/bpf_percpu_obj_drop_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_percpu_obj_drop_impl </span></a>
- <a href="../../kfuncs/bpf_refcount_acquire_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_refcount_acquire_impl </span></a>
- <a href="../../kfuncs/bpf_list_push_front_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_list_push_front_impl </span></a>
- <a href="../../kfuncs/bpf_list_push_back_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_list_push_back_impl </span></a>
- <a href="../../kfuncs/bpf_list_pop_front/" class="md-nav__link"><span class="md-ellipsis"> bpf_list_pop_front </span></a>
- <a href="../../kfuncs/bpf_list_pop_back/" class="md-nav__link"><span class="md-ellipsis"> bpf_list_pop_back </span></a>
- <a href="../../kfuncs/bpf_list_back/" class="md-nav__link"><span class="md-ellipsis"> bpf_list_back </span></a>
- <a href="../../kfuncs/bpf_list_front/" class="md-nav__link"><span class="md-ellipsis"> bpf_list_front </span></a>

<span class="md-ellipsis"> BPF Arena KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF Arena KFuncs

- <a href="../../kfuncs/bpf_arena_alloc_pages/" class="md-nav__link"><span class="md-ellipsis"> bpf_arena_alloc_pages </span></a>
- <a href="../../kfuncs/bpf_arena_free_pages/" class="md-nav__link"><span class="md-ellipsis"> bpf_arena_free_pages </span></a>
- <a href="../../kfuncs/bpf_arena_reserve_pages/" class="md-nav__link"><span class="md-ellipsis"> bpf_arena_reserve_pages </span></a>

<span class="md-ellipsis"> BPF task KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF task KFuncs

- <a href="../../kfuncs/bpf_task_acquire/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_acquire </span></a>
- <a href="../../kfuncs/bpf_task_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_release </span></a>
- <a href="../../kfuncs/bpf_send_signal_task/" class="md-nav__link"><span class="md-ellipsis"> bpf_send_signal_task </span></a>

<span class="md-ellipsis"> BPF Red-Black-tree KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF Red-Black-tree KFuncs

- <a href="../../kfuncs/bpf_rbtree_add_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_add_impl </span></a>
- <a href="../../kfuncs/bpf_rbtree_first/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_first </span></a>
- <a href="../../kfuncs/bpf_rbtree_remove/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_remove </span></a>
- <a href="../../kfuncs/bpf_rbtree_left/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_left </span></a>
- <a href="../../kfuncs/bpf_rbtree_right/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_right </span></a>
- <a href="../../kfuncs/bpf_rbtree_root/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_root </span></a>

<span class="md-ellipsis"> Kfuncs for acquiring and releasing cGroup references </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for acquiring and releasing cGroup references

- <a href="../../kfuncs/bpf_cgroup_acquire/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgroup_acquire </span></a>
- <a href="../../kfuncs/bpf_cgroup_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgroup_release </span></a>
- <a href="../../kfuncs/bpf_cgroup_ancestor/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgroup_ancestor </span></a>
- <a href="../../kfuncs/bpf_cgroup_from_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgroup_from_id </span></a>

<span class="md-ellipsis"> Kfuncs for querying tasks </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for querying tasks

- <a href="../../kfuncs/bpf_task_under_cgroup/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_under_cgroup </span></a>
- <a href="../../kfuncs/bpf_task_get_cgroup1/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_get_cgroup1 </span></a>
- <a href="../../kfuncs/bpf_task_from_pid/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_from_pid </span></a>
- <a href="../../kfuncs/bpf_task_from_vpid/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_from_vpid </span></a>

<span class="md-ellipsis"> KFuncs for memory allocator inspection </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> KFuncs for memory allocator inspection

- <a href="../../kfuncs/bpf_get_kmem_cache/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_kmem_cache </span></a>

<span class="md-ellipsis"> Kfuncs for casting pointers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for casting pointers

- <a href="../../kfuncs/bpf_cast_to_kern_ctx/" class="md-nav__link"><span class="md-ellipsis"> bpf_cast_to_kern_ctx </span></a>
- <a href="../../kfuncs/bpf_rdonly_cast/" class="md-nav__link"><span class="md-ellipsis"> bpf_rdonly_cast </span></a>

<span class="md-ellipsis"> Kfuncs for taking and releasing RCU read locks </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for taking and releasing RCU read locks

- <a href="../../kfuncs/bpf_rcu_read_lock/" class="md-nav__link"><span class="md-ellipsis"> bpf_rcu_read_lock </span></a>
- <a href="../../kfuncs/bpf_rcu_read_unlock/" class="md-nav__link"><span class="md-ellipsis"> bpf_rcu_read_unlock </span></a>

<span class="md-ellipsis"> Kfuncs for dynamic pointer slices </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for dynamic pointer slices

- <a href="../../kfuncs/bpf_dynptr_slice/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_slice </span></a>
- <a href="../../kfuncs/bpf_dynptr_slice_rdwr/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_slice_rdwr </span></a>

<span class="md-ellipsis"> Open coded iterator </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Open coded iterator

<span class="md-ellipsis"> Kfuncs for open coded numeric iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for open coded numeric iterators

- <a href="../../kfuncs/bpf_iter_num_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_num_new </span></a>
- <a href="../../kfuncs/bpf_iter_num_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_num_next </span></a>
- <a href="../../kfuncs/bpf_iter_num_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_num_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for open coded virtual memory area iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for open coded virtual memory area iterators

- <a href="../../kfuncs/bpf_iter_task_vma_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_task_vma_new </span></a>
- <a href="../../kfuncs/bpf_iter_task_vma_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_task_vma_next </span></a>
- <a href="../../kfuncs/bpf_iter_task_vma_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_task_vma_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for bits </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for bits

- <a href="../../kfuncs/bpf_iter_bits_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_bits_new </span></a>
- <a href="../../kfuncs/bpf_iter_bits_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_bits_next </span></a>
- <a href="../../kfuncs/bpf_iter_bits_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_bits_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for open coded task cGroup iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for open coded task cGroup iterators

- <a href="../../kfuncs/bpf_iter_css_task_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_css_task_new </span></a>
- <a href="../../kfuncs/bpf_iter_css_task_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_css_task_next </span></a>
- <a href="../../kfuncs/bpf_iter_css_task_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_css_task_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for open coded cGroup iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for open coded cGroup iterators

- <a href="../../kfuncs/bpf_iter_css_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_css_new </span></a>
- <a href="../../kfuncs/bpf_iter_css_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_css_next </span></a>
- <a href="../../kfuncs/bpf_iter_css_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_css_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for open coded task iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for open coded task iterators

- <a href="../../kfuncs/bpf_iter_task_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_task_new </span></a>
- <a href="../../kfuncs/bpf_iter_task_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_task_next </span></a>
- <a href="../../kfuncs/bpf_iter_task_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_task_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for slab memory allocation iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for slab memory allocation iterators

- <a href="../../kfuncs/bpf_iter_kmem_cache_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_kmem_cache_new </span></a>
- <a href="../../kfuncs/bpf_iter_kmem_cache_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_kmem_cache_next </span></a>
- <a href="../../kfuncs/bpf_iter_kmem_cache_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_kmem_cache_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for sched_ext dispatch queue iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for sched_ext dispatch queue iterators

- <a href="../../kfuncs/bpf_iter_scx_dsq_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_scx_dsq_new </span></a>
- <a href="../../kfuncs/bpf_iter_scx_dsq_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_scx_dsq_next </span></a>
- <a href="../../kfuncs/bpf_iter_scx_dsq_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_scx_dsq_destroy </span></a>

<span class="md-ellipsis"> Kfuncs for dynamic pointers </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for dynamic pointers

- <a href="../../kfuncs/bpf_dynptr_adjust/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_adjust </span></a>
- <a href="../../kfuncs/bpf_dynptr_is_null/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_is_null </span></a>
- <a href="../../kfuncs/bpf_dynptr_is_rdonly/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_is_rdonly </span></a>
- <a href="../../kfuncs/bpf_dynptr_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_size </span></a>
- <a href="../../kfuncs/bpf_dynptr_clone/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_clone </span></a>
- <a href="../../kfuncs/bpf_dynptr_copy/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_copy </span></a>
- <a href="../../kfuncs/bpf_dynptr_memset/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_memset </span></a>

<span class="md-ellipsis"> Kfuncs for DMA buffer iterators </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Kfuncs for DMA buffer iterators

- <a href="../../kfuncs/bpf_iter_dmabuf_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_dmabuf_new </span></a>
- <a href="../../kfuncs/bpf_iter_dmabuf_next/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_dmabuf_next </span></a>
- <a href="../../kfuncs/bpf_iter_dmabuf_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_dmabuf_destroy </span></a>

<span class="md-ellipsis"> Misc KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Misc KFuncs

- <a href="../../kfuncs/bpf_map_sum_elem_count/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_sum_elem_count </span></a>
- <a href="../../kfuncs/bpf_get_fsverity_digest/" class="md-nav__link"><span class="md-ellipsis"> bpf_get_fsverity_digest </span></a>
- <a href="../../kfuncs/__bpf_trap/" class="md-nav__link"><span class="md-ellipsis"> __bpf_trap </span></a>

<span class="md-ellipsis"> Preemption kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Preemption kfuncs

- <a href="../../kfuncs/bpf_preempt_disable/" class="md-nav__link"><span class="md-ellipsis"> bpf_preempt_disable </span></a>
- <a href="../../kfuncs/bpf_preempt_enable/" class="md-nav__link"><span class="md-ellipsis"> bpf_preempt_enable </span></a>

<span class="md-ellipsis"> Work-queue KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Work-queue KFuncs

- <a href="../../kfuncs/bpf_wq_init/" class="md-nav__link"><span class="md-ellipsis"> bpf_wq_init </span></a>
- <a href="../../kfuncs/bpf_wq_set_callback_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_wq_set_callback_impl </span></a>
- <a href="../../kfuncs/bpf_wq_start/" class="md-nav__link"><span class="md-ellipsis"> bpf_wq_start </span></a>

<span class="md-ellipsis"> XDP metadata kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> XDP metadata kfuncs

- <a href="../../kfuncs/bpf_xdp_metadata_rx_timestamp/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_metadata_rx_timestamp </span></a>
- <a href="../../kfuncs/bpf_xdp_metadata_rx_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_metadata_rx_hash </span></a>
- <a href="../../kfuncs/bpf_xdp_metadata_rx_vlan_tag/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_metadata_rx_vlan_tag </span></a>

<span class="md-ellipsis"> XDP/SKB dynamic pointer kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> XDP/SKB dynamic pointer kfuncs

- <a href="../../kfuncs/bpf_dynptr_from_skb/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_from_skb </span></a>
- <a href="../../kfuncs/bpf_dynptr_from_xdp/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_from_xdp </span></a>
- <a href="../../kfuncs/bpf_dynptr_from_skb_meta/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_from_skb_meta </span></a>

<span class="md-ellipsis"> Socket related kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Socket related kfuncs

- <a href="../../kfuncs/bpf_sock_addr_set_sun_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_addr_set_sun_path </span></a>
- <a href="../../kfuncs/bpf_sock_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_destroy </span></a>

<span class="md-ellipsis"> Network crypto kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Network crypto kfuncs

- <a href="../../kfuncs/bpf_crypto_ctx_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_crypto_ctx_create </span></a>
- <a href="../../kfuncs/bpf_crypto_ctx_acquire/" class="md-nav__link"><span class="md-ellipsis"> bpf_crypto_ctx_acquire </span></a>
- <a href="../../kfuncs/bpf_crypto_ctx_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_crypto_ctx_release </span></a>
- <a href="../../kfuncs/bpf_crypto_decrypt/" class="md-nav__link"><span class="md-ellipsis"> bpf_crypto_decrypt </span></a>
- <a href="../../kfuncs/bpf_crypto_encrypt/" class="md-nav__link"><span class="md-ellipsis"> bpf_crypto_encrypt </span></a>

<span class="md-ellipsis"> BBR congestion control kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BBR congestion control kfuncs

- <a href="../../kfuncs/bbr_init/" class="md-nav__link"><span class="md-ellipsis"> bbr_init </span></a>
- <a href="../../kfuncs/bbr_main/" class="md-nav__link"><span class="md-ellipsis"> bbr_main </span></a>
- <a href="../../kfuncs/bbr_sndbuf_expand/" class="md-nav__link"><span class="md-ellipsis"> bbr_sndbuf_expand </span></a>
- <a href="../../kfuncs/bbr_undo_cwnd/" class="md-nav__link"><span class="md-ellipsis"> bbr_undo_cwnd </span></a>
- <a href="../../kfuncs/bbr_cwnd_event/" class="md-nav__link"><span class="md-ellipsis"> bbr_cwnd_event </span></a>
- <a href="../../kfuncs/bbr_ssthresh/" class="md-nav__link"><span class="md-ellipsis"> bbr_ssthresh </span></a>
- <a href="../../kfuncs/bbr_min_tso_segs/" class="md-nav__link"><span class="md-ellipsis"> bbr_min_tso_segs </span></a>
- <a href="../../kfuncs/bbr_set_state/" class="md-nav__link"><span class="md-ellipsis"> bbr_set_state </span></a>

<span class="md-ellipsis"> Cubic TCP congestion control kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Cubic TCP congestion control kfuncs

- <a href="../../kfuncs/cubictcp_init/" class="md-nav__link"><span class="md-ellipsis"> cubictcp_init </span></a>
- <a href="../../kfuncs/cubictcp_recalc_ssthresh/" class="md-nav__link"><span class="md-ellipsis"> cubictcp_recalc_ssthresh </span></a>
- <a href="../../kfuncs/cubictcp_cong_avoid/" class="md-nav__link"><span class="md-ellipsis"> cubictcp_cong_avoid </span></a>
- <a href="../../kfuncs/cubictcp_state/" class="md-nav__link"><span class="md-ellipsis"> cubictcp_state </span></a>
- <a href="../../kfuncs/cubictcp_cwnd_event/" class="md-nav__link"><span class="md-ellipsis"> cubictcp_cwnd_event </span></a>
- <a href="../../kfuncs/cubictcp_acked/" class="md-nav__link"><span class="md-ellipsis"> cubictcp_acked </span></a>

<span class="md-ellipsis"> DC TCP congestion control kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> DC TCP congestion control kfuncs

- <a href="../../kfuncs/dctcp_init/" class="md-nav__link"><span class="md-ellipsis"> dctcp_init </span></a>
- <a href="../../kfuncs/dctcp_update_alpha/" class="md-nav__link"><span class="md-ellipsis"> dctcp_update_alpha </span></a>
- <a href="../../kfuncs/dctcp_cwnd_event/" class="md-nav__link"><span class="md-ellipsis"> dctcp_cwnd_event </span></a>
- <a href="../../kfuncs/dctcp_ssthresh/" class="md-nav__link"><span class="md-ellipsis"> dctcp_ssthresh </span></a>
- <a href="../../kfuncs/dctcp_cwnd_undo/" class="md-nav__link"><span class="md-ellipsis"> dctcp_cwnd_undo </span></a>
- <a href="../../kfuncs/dctcp_state/" class="md-nav__link"><span class="md-ellipsis"> dctcp_state </span></a>

<span class="md-ellipsis"> TCP Reno congestion control kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> TCP Reno congestion control kfuncs

- <a href="../../kfuncs/tcp_reno_ssthresh/" class="md-nav__link"><span class="md-ellipsis"> tcp_reno_ssthresh </span></a>
- <a href="../../kfuncs/tcp_reno_cong_avoid/" class="md-nav__link"><span class="md-ellipsis"> tcp_reno_cong_avoid </span></a>
- <a href="../../kfuncs/tcp_reno_undo_cwnd/" class="md-nav__link"><span class="md-ellipsis"> tcp_reno_undo_cwnd </span></a>
- <a href="../../kfuncs/tcp_slow_start/" class="md-nav__link"><span class="md-ellipsis"> tcp_slow_start </span></a>
- <a href="../../kfuncs/tcp_cong_avoid_ai/" class="md-nav__link"><span class="md-ellipsis"> tcp_cong_avoid_ai </span></a>

<span class="md-ellipsis"> Foo over UDP KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Foo over UDP KFuncs

- <a href="../../kfuncs/bpf_skb_set_fou_encap/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_set_fou_encap </span></a>
- <a href="../../kfuncs/bpf_skb_get_fou_encap/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_get_fou_encap </span></a>

<span class="md-ellipsis"> SYN Cookie KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> SYN Cookie KFuncs

- <a href="../../kfuncs/bpf_sk_assign_tcp_reqsk/" class="md-nav__link"><span class="md-ellipsis"> bpf_sk_assign_tcp_reqsk </span></a>

<span class="md-ellipsis"> Connection tracking KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Connection tracking KFuncs

- <a href="../../kfuncs/bpf_ct_set_nat_info/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_set_nat_info </span></a>
- <a href="../../kfuncs/bpf_xdp_ct_alloc/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_ct_alloc </span></a>
- <a href="../../kfuncs/bpf_xdp_ct_lookup/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_ct_lookup </span></a>
- <a href="../../kfuncs/bpf_skb_ct_alloc/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_ct_alloc </span></a>
- <a href="../../kfuncs/bpf_skb_ct_lookup/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_ct_lookup </span></a>
- <a href="../../kfuncs/bpf_ct_insert_entry/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_insert_entry </span></a>
- <a href="../../kfuncs/bpf_ct_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_release </span></a>
- <a href="../../kfuncs/bpf_ct_set_timeout/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_set_timeout </span></a>
- <a href="../../kfuncs/bpf_ct_change_timeout/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_change_timeout </span></a>
- <a href="../../kfuncs/bpf_ct_set_status/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_set_status </span></a>
- <a href="../../kfuncs/bpf_ct_change_status/" class="md-nav__link"><span class="md-ellipsis"> bpf_ct_change_status </span></a>

<span class="md-ellipsis"> XDP KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> XDP KFuncs

- <a href="../../kfuncs/bpf_xdp_flow_lookup/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_flow_lookup </span></a>
- <a href="../../kfuncs/bpf_xdp_pull_data/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_pull_data </span></a>

<span class="md-ellipsis"> XFRM KFuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> XFRM KFuncs

- <a href="../../kfuncs/bpf_skb_get_xfrm_info/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_get_xfrm_info </span></a>
- <a href="../../kfuncs/bpf_skb_set_xfrm_info/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_set_xfrm_info </span></a>
- <a href="../../kfuncs/bpf_xdp_get_xfrm_state/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_get_xfrm_state </span></a>
- <a href="../../kfuncs/bpf_xdp_xfrm_state_release/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_xfrm_state_release </span></a>

<span class="md-ellipsis"> HID Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> HID Kfuncs

- <a href="../../kfuncs/hid_bpf_get_data/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_get_data </span></a>
- <a href="../../kfuncs/hid_bpf_attach_prog/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_attach_prog </span></a>
- <a href="../../kfuncs/hid_bpf_allocate_context/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_allocate_context </span></a>
- <a href="../../kfuncs/hid_bpf_release_context/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_release_context </span></a>
- <a href="../../kfuncs/hid_bpf_hw_request/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_hw_request </span></a>
- <a href="../../kfuncs/hid_bpf_hw_output_report/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_hw_output_report </span></a>
- <a href="../../kfuncs/hid_bpf_input_report/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_input_report </span></a>
- <a href="../../kfuncs/hid_bpf_try_input_report/" class="md-nav__link"><span class="md-ellipsis"> hid_bpf_try_input_report </span></a>

<span class="md-ellipsis"> KProbe session Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> KProbe session Kfuncs

- <a href="../../kfuncs/bpf_session_cookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_session_cookie </span></a>
- <a href="../../kfuncs/bpf_session_is_return/" class="md-nav__link"><span class="md-ellipsis"> bpf_session_is_return </span></a>

<span class="md-ellipsis"> Memory probe Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Memory probe Kfuncs

- <a href="../../kfuncs/bpf_copy_from_user_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_str </span></a>
- <a href="../../kfuncs/bpf_copy_from_user_task_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_task_str </span></a>

<span class="md-ellipsis"> IRQ Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> IRQ Kfuncs

- <a href="../../kfuncs/bpf_local_irq_save/" class="md-nav__link"><span class="md-ellipsis"> bpf_local_irq_save </span></a>
- <a href="../../kfuncs/bpf_local_irq_restore/" class="md-nav__link"><span class="md-ellipsis"> bpf_local_irq_restore </span></a>

<span class="md-ellipsis"> sched_ext Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> sched_ext Kfuncs

<a href="../../kfuncs/scx_bpf_kick_cpu/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_kick_cpu </span></a>

<a href="../../kfuncs/scx_bpf_select_cpu_dfl/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_select_cpu_dfl </span></a>

<a href="../../kfuncs/scx_bpf_select_cpu_and/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_select_cpu_and </span></a>

<a href="../../kfuncs/__scx_bpf_select_cpu_and/" class="md-nav__link"><span class="md-ellipsis"> __scx_bpf_select_cpu_and </span></a>

<a href="../../kfuncs/scx_bpf_cpu_rq/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_cpu_rq </span></a>

<a href="../../kfuncs/scx_bpf_now/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_now </span></a>

<a href="../../kfuncs/scx_bpf_cpu_curr/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_cpu_curr </span></a>

<a href="../../kfuncs/scx_bpf_locked_rq/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_locked_rq </span></a>

<span class="md-ellipsis"> Dispatch Queue Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Dispatch Queue Kfuncs

- <a href="../../kfuncs/scx_bpf_create_dsq/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_create_dsq </span></a>
- <a href="../../kfuncs/scx_bpf_destroy_dsq/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_destroy_dsq </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_nr_queued/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_nr_queued </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_insert/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_insert </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_insert___v2/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_insert___v2 </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_insert_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_insert_vtime </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_vtime </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_move_to_local/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_move_to_local </span></a>
- <a href="../../kfuncs/scx_bpf_consume/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_consume </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_move_set_slice/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_move_set_slice </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch_from_dsq_set_slice/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_from_dsq_set_slice </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_move_set_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_move_set_vtime </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch_from_dsq_set_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_from_dsq_set_vtime </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_move/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_move </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch_from_dsq/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_from_dsq </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_move_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_move_vtime </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch_vtime_from_dsq/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_vtime_from_dsq </span></a>
- <a href="../../kfuncs/scx_bpf_reenqueue_local/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_reenqueue_local </span></a>
- <a href="../../kfuncs/scx_bpf_reenqueue_local___v2/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_reenqueue_local___v2 </span></a>
- <a href="../../kfuncs/scx_bpf_dsq_peek/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_peek </span></a>

<span class="md-ellipsis"> Dispatch Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Dispatch Kfuncs

- <a href="../../kfuncs/scx_bpf_dispatch_nr_slots/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_nr_slots </span></a>
- <a href="../../kfuncs/scx_bpf_dispatch_cancel/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dispatch_cancel </span></a>

<span class="md-ellipsis"> Error and debug Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Error and debug Kfuncs

- <a href="../../kfuncs/scx_bpf_exit_bstr/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_exit_bstr </span></a>
- <a href="../../kfuncs/scx_bpf_error_bstr/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_error_bstr </span></a>
- <a href="../../kfuncs/scx_bpf_dump_bstr/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dump_bstr </span></a>

<span class="md-ellipsis"> CPU performance Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CPU performance Kfuncs

- <a href="../../kfuncs/scx_bpf_cpuperf_cap/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_cpuperf_cap </span></a>
- <a href="../../kfuncs/scx_bpf_cpuperf_cur/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_cpuperf_cur </span></a>
- <a href="../../kfuncs/scx_bpf_cpuperf_set/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_cpuperf_set </span></a>
- <a href="../../kfuncs/scx_bpf_nr_cpu_ids/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_nr_cpu_ids </span></a>

<span class="md-ellipsis"> CPU mask Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CPU mask Kfuncs

- <a href="../../kfuncs/scx_bpf_get_possible_cpumask/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_get_possible_cpumask </span></a>
- <a href="../../kfuncs/scx_bpf_get_online_cpumask/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_get_online_cpumask </span></a>
- <a href="../../kfuncs/scx_bpf_put_cpumask/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_put_cpumask </span></a>

<span class="md-ellipsis"> Idle CPU mask Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Idle CPU mask Kfuncs

- <a href="../../kfuncs/scx_bpf_get_idle_cpumask/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_get_idle_cpumask </span></a>
- <a href="../../kfuncs/scx_bpf_get_idle_smtmask/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_get_idle_smtmask </span></a>
- <a href="../../kfuncs/scx_bpf_put_idle_cpumask/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_put_idle_cpumask </span></a>
- <a href="../../kfuncs/scx_bpf_test_and_clear_cpu_idle/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_test_and_clear_cpu_idle </span></a>
- <a href="../../kfuncs/scx_bpf_pick_idle_cpu/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_pick_idle_cpu </span></a>
- <a href="../../kfuncs/scx_bpf_pick_any_cpu/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_pick_any_cpu </span></a>

<span class="md-ellipsis"> Task Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Task Kfuncs

- <a href="../../kfuncs/scx_bpf_task_running/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_task_running </span></a>
- <a href="../../kfuncs/scx_bpf_task_cpu/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_task_cpu </span></a>
- <a href="../../kfuncs/scx_bpf_task_cgroup/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_task_cgroup </span></a>
- <a href="../../kfuncs/scx_bpf_task_set_slice/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_task_set_slice </span></a>
- <a href="../../kfuncs/scx_bpf_task_set_dsq_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_task_set_dsq_vtime </span></a>

<span class="md-ellipsis"> NUMA Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> NUMA Kfuncs

- <a href="../../kfuncs/scx_bpf_cpu_node/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_cpu_node </span></a>
- <a href="../../kfuncs/scx_bpf_nr_node_ids/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_nr_node_ids </span></a>
- <a href="../../kfuncs/scx_bpf_pick_any_cpu_node/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_pick_any_cpu_node </span></a>
- <a href="../../kfuncs/scx_bpf_pick_idle_cpu_node/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_pick_idle_cpu_node </span></a>

<span class="md-ellipsis"> Resilient Queued spinlock Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Resilient Queued spinlock Kfuncs

- <a href="../../kfuncs/bpf_res_spin_lock/" class="md-nav__link"><span class="md-ellipsis"> bpf_res_spin_lock </span></a>
- <a href="../../kfuncs/bpf_res_spin_lock_irqsave/" class="md-nav__link"><span class="md-ellipsis"> bpf_res_spin_lock_irqsave </span></a>
- <a href="../../kfuncs/bpf_res_spin_unlock/" class="md-nav__link"><span class="md-ellipsis"> bpf_res_spin_unlock </span></a>
- <a href="../../kfuncs/bpf_res_spin_unlock_irqrestore/" class="md-nav__link"><span class="md-ellipsis"> bpf_res_spin_unlock_irqrestore </span></a>

<span class="md-ellipsis"> Sock ops Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Sock ops Kfuncs

- <a href="../../kfuncs/bpf_sock_ops_enable_tx_tstamp/" class="md-nav__link"><span class="md-ellipsis"> bpf_sock_ops_enable_tx_tstamp </span></a>

<span class="md-ellipsis"> Memory probe to dynptr Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Memory probe to dynptr Kfuncs

- <a href="../../kfuncs/bpf_probe_read_user_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_user_dynptr </span></a>
- <a href="../../kfuncs/bpf_probe_read_kernel_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_kernel_dynptr </span></a>
- <a href="../../kfuncs/bpf_probe_read_user_str_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_user_str_dynptr </span></a>
- <a href="../../kfuncs/bpf_probe_read_kernel_str_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_probe_read_kernel_str_dynptr </span></a>
- <a href="../../kfuncs/bpf_copy_from_user_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_dynptr </span></a>
- <a href="../../kfuncs/bpf_copy_from_user_str_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_str_dynptr </span></a>
- <a href="../../kfuncs/bpf_copy_from_user_task_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_task_dynptr </span></a>
- <a href="../../kfuncs/bpf_copy_from_user_task_str_dynptr/" class="md-nav__link"><span class="md-ellipsis"> bpf_copy_from_user_task_str_dynptr </span></a>

<span class="md-ellipsis"> File dynptr Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> File dynptr Kfuncs

- <a href="../../kfuncs/bpf_dynptr_from_file/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_from_file </span></a>
- <a href="../../kfuncs/bpf_dynptr_file_discard/" class="md-nav__link"><span class="md-ellipsis"> bpf_dynptr_file_discard </span></a>

<span class="md-ellipsis"> BPF Qdisc kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF Qdisc kfuncs

- <a href="../../kfuncs/bpf_kfree_skb/" class="md-nav__link"><span class="md-ellipsis"> bpf_kfree_skb </span></a>
- <a href="../../kfuncs/bpf_qdisc_bstats_update/" class="md-nav__link"><span class="md-ellipsis"> bpf_qdisc_bstats_update </span></a>
- <a href="../../kfuncs/bpf_qdisc_init_prologue/" class="md-nav__link"><span class="md-ellipsis"> bpf_qdisc_init_prologue </span></a>
- <a href="../../kfuncs/bpf_qdisc_reset_destroy_epilogue/" class="md-nav__link"><span class="md-ellipsis"> bpf_qdisc_reset_destroy_epilogue </span></a>
- <a href="../../kfuncs/bpf_qdisc_skb_drop/" class="md-nav__link"><span class="md-ellipsis"> bpf_qdisc_skb_drop </span></a>
- <a href="../../kfuncs/bpf_qdisc_watchdog_schedule/" class="md-nav__link"><span class="md-ellipsis"> bpf_qdisc_watchdog_schedule </span></a>
- <a href="../../kfuncs/bpf_skb_get_hash/" class="md-nav__link"><span class="md-ellipsis"> bpf_skb_get_hash </span></a>

<span class="md-ellipsis"> String Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> String Kfuncs

- <a href="../../kfuncs/bpf_strchr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strchr </span></a>
- <a href="../../kfuncs/bpf_strchrnul/" class="md-nav__link"><span class="md-ellipsis"> bpf_strchrnul </span></a>
- <a href="../../kfuncs/bpf_strcmp/" class="md-nav__link"><span class="md-ellipsis"> bpf_strcmp </span></a>
- <a href="../../kfuncs/bpf_strcspn/" class="md-nav__link"><span class="md-ellipsis"> bpf_strcspn </span></a>
- <a href="../../kfuncs/bpf_strlen/" class="md-nav__link"><span class="md-ellipsis"> bpf_strlen </span></a>
- <a href="../../kfuncs/bpf_strnchr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strnchr </span></a>
- <a href="../../kfuncs/bpf_strnlen/" class="md-nav__link"><span class="md-ellipsis"> bpf_strnlen </span></a>
- <a href="../../kfuncs/bpf_strnstr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strnstr </span></a>
- <a href="../../kfuncs/bpf_strrchr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strrchr </span></a>
- <a href="../../kfuncs/bpf_strspn/" class="md-nav__link"><span class="md-ellipsis"> bpf_strspn </span></a>
- <a href="../../kfuncs/bpf_strstr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strstr </span></a>
- <a href="../../kfuncs/bpf_strcasecmp/" class="md-nav__link"><span class="md-ellipsis"> bpf_strcasecmp </span></a>
- <a href="../../kfuncs/bpf_strcasestr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strcasestr </span></a>
- <a href="../../kfuncs/bpf_strncasestr/" class="md-nav__link"><span class="md-ellipsis"> bpf_strncasestr </span></a>

<span class="md-ellipsis"> Debug stream Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Debug stream Kfuncs

- <a href="../../kfuncs/bpf_stream_vprintk_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_stream_vprintk_impl </span></a>

<span class="md-ellipsis"> CGroup xattr Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CGroup xattr Kfuncs

- <a href="../../kfuncs/bpf_cgroup_read_xattr/" class="md-nav__link"><span class="md-ellipsis"> bpf_cgroup_read_xattr </span></a>

<span class="md-ellipsis"> Task work schedule Kfuncs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Task work schedule Kfuncs

- <a href="../../kfuncs/bpf_task_work_schedule_resume_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_work_schedule_resume_impl </span></a>
- <a href="../../kfuncs/bpf_task_work_schedule_signal_impl/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_work_schedule_signal_impl </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../timeline/" class="md-nav__link"><span class="md-ellipsis"> eBPF Timeline </span></a>

</div>

<span class="md-nav__icon md-icon"></span> eBPF Timeline

<div class="md-nav__link md-nav__container">

<a href="../../../ebpf-library/" class="md-nav__link"><span class="md-ellipsis"> eBPF libraries </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> eBPF libraries

<div class="md-nav__link md-nav__container">

<a href="../../../ebpf-library/libbpf/" class="md-nav__link"><span class="md-ellipsis"> Libbpf </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Libbpf

<div class="md-nav__link md-nav__container">

<a href="../../../ebpf-library/libbpf/userspace/" class="md-nav__link"><span class="md-ellipsis"> Userspace </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Userspace

<span class="md-ellipsis"> BPF Object functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF Object functions

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__open/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__open </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__open_file/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__open_file </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__open_mem/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__open_mem </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__load/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__load </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__close/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__close </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__pin_maps/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__pin_maps </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__unpin_maps/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__unpin_maps </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__pin_programs/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__pin_programs </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__unpin_programs/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__unpin_programs </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__pin/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__pin </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__unpin/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__unpin </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__name/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__name </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__kversion/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__kversion </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__set_kversion/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__set_kversion </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__token_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__token_fd </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__btf/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__btf </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__btf_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__btf_fd </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__find_program_by_name/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__find_program_by_name </span></a>

<span class="md-ellipsis"> BPF Skeleton functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF Skeleton functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__open_skeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__open_skeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__load_skeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__load_skeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__attach_skeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__attach_skeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__detach_skeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__detach_skeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__destroy_skeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__destroy_skeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__open_subskeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__open_subskeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__destroy_subskeleton/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__destroy_subskeleton </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_object__gen_loader/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__gen_loader </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__next_program/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__next_program </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__prev_program/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__prev_program </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__find_map_by_name/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__find_map_by_name </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__find_map_fd_by_name/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__find_map_fd_by_name </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__next_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__next_map </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_object__prev_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_object__prev_map </span></a>

<span class="md-ellipsis"> BPF Program functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BPF Program functions

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_ifindex/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_ifindex </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__name/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__name </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__section_name/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__section_name </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__autoload/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__autoload </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_autoload/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_autoload </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__autoattach/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__autoattach </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_autoattach/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_autoattach </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__insns/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__insns </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_insns/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_insns </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__insn_cnt/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__insn_cnt </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__fd </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__pin/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__pin </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__unpin/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__unpin </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__unload/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__unload </span></a>

<span class="md-ellipsis"> Program attach functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Program attach functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_perf_event/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_perf_event </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_perf_event_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_perf_event_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_kprobe/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_kprobe </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_kprobe_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_kprobe_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_kprobe_multi_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_kprobe_multi_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_uprobe_multi/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_uprobe_multi </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_ksyscall/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_ksyscall </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_uprobe/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_uprobe </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_uprobe_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_uprobe_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_usdt/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_usdt </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_tracepoint/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_tracepoint </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_tracepoint_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_tracepoint_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_raw_tracepoint/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_raw_tracepoint </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_raw_tracepoint_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_raw_tracepoint_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_trace/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_trace </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_trace_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_trace_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_lsm/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_lsm </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_cgroup/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_cgroup </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_netns/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_netns </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_sockmap/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_sockmap </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_xdp/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_xdp </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_freplace/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_freplace </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_netfilter/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_netfilter </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_tcx/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_tcx </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_netkit/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_netkit </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__attach_iter/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__attach_iter </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__type/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__type </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_type </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_expected_attach_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_expected_attach_type </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__flags </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_flags </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__log_level/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__log_level </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_log_level/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_log_level </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__log_buf/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__log_buf </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_log_buf/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_log_buf </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__set_attach_target/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__set_attach_target </span></a>

<a href="../../../ebpf-library/libbpf/userspace/bpf_program__expected_attach_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__expected_attach_type </span></a>

<span class="md-ellipsis"> Link functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Link functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__open/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__open </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__pin_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__pin_path </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__pin/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__pin </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__unpin/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__unpin </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__update_program/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__update_program </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__disconnect/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__disconnect </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__detach/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__detach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__destroy </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link__update_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_link__update_map </span></a>

<span class="md-ellipsis"> Map functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Map functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__attach_struct_ops/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__attach_struct_ops </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_autocreate/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_autocreate </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__autocreate/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__autocreate </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_autoattach/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_autoattach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__autoattach/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__autoattach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__reuse_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__reuse_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__name/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__name </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__type/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__max_entries/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__max_entries </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_max_entries/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_max_entries </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__map_flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__map_flags </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_map_flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_map_flags </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__numa_node/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__numa_node </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_numa_node/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_numa_node </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__key_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__key_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_key_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_key_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__value_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__value_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_value_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_value_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__btf_key_type_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__btf_key_type_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__btf_value_type_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__btf_value_type_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__ifindex/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__ifindex </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_ifindex/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_ifindex </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__map_extra/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__map_extra </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_map_extra/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_map_extra </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_initial_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_initial_value </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__initial_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__initial_value </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__is_internal/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__is_internal </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_pin_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_pin_path </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__pin_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__pin_path </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__is_pinned/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__is_pinned </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__pin/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__pin </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__unpin/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__unpin </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__set_inner_map_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__set_inner_map_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__inner_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__inner_map </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__lookup_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__lookup_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__update_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__update_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__delete_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__delete_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__lookup_and_delete_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__lookup_and_delete_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__get_next_key/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__get_next_key </span></a>

<span class="md-ellipsis"> XDP functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> XDP functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_xdp_attach/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_attach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_xdp_detach/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_detach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_xdp_query/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_query </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_xdp_query_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_xdp_query_id </span></a>

<span class="md-ellipsis"> TC functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> TC functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_tc_hook_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_tc_hook_create </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_tc_hook_destroy/" class="md-nav__link"><span class="md-ellipsis"> bpf_tc_hook_destroy </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_tc_attach/" class="md-nav__link"><span class="md-ellipsis"> bpf_tc_attach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_tc_detach/" class="md-nav__link"><span class="md-ellipsis"> bpf_tc_detach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_tc_query/" class="md-nav__link"><span class="md-ellipsis"> bpf_tc_query </span></a>

<span class="md-ellipsis"> Ring buffer manager functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Ring buffer manager functions

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__new/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__new </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__free/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__free </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__add/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__add </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__poll/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__poll </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__consume/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__consume </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__consume_n/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__consume_n </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__epoll_fd/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__epoll_fd </span></a>

<a href="../../../ebpf-library/libbpf/userspace/ring_buffer__ring/" class="md-nav__link"><span class="md-ellipsis"> ring_buffer__ring </span></a>

<span class="md-ellipsis"> Ring buffer functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Ring buffer functions

- <a href="../../../ebpf-library/libbpf/userspace/ring__consumer_pos/" class="md-nav__link"><span class="md-ellipsis"> ring__consumer_pos </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/ring__producer_pos/" class="md-nav__link"><span class="md-ellipsis"> ring__producer_pos </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/ring__avail_data_size/" class="md-nav__link"><span class="md-ellipsis"> ring__avail_data_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/ring__size/" class="md-nav__link"><span class="md-ellipsis"> ring__size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/ring__map_fd/" class="md-nav__link"><span class="md-ellipsis"> ring__map_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/ring__consume/" class="md-nav__link"><span class="md-ellipsis"> ring__consume </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/ring__consume_n/" class="md-nav__link"><span class="md-ellipsis"> ring__consume_n </span></a>

<span class="md-ellipsis"> User ring buffer </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> User ring buffer

- <a href="../../../ebpf-library/libbpf/userspace/user_ring_buffer__new/" class="md-nav__link"><span class="md-ellipsis"> user_ring_buffer__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/user_ring_buffer__reserve/" class="md-nav__link"><span class="md-ellipsis"> user_ring_buffer__reserve </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/user_ring_buffer__reserve_blocking/" class="md-nav__link"><span class="md-ellipsis"> user_ring_buffer__reserve_blocking </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/user_ring_buffer__submit/" class="md-nav__link"><span class="md-ellipsis"> user_ring_buffer__submit </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/user_ring_buffer__discard/" class="md-nav__link"><span class="md-ellipsis"> user_ring_buffer__discard </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/user_ring_buffer__free/" class="md-nav__link"><span class="md-ellipsis"> user_ring_buffer__free </span></a>

<span class="md-ellipsis"> Perf buffer functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Perf buffer functions

- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__new/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__new_raw/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__new_raw </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__free/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__free </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__epoll_fd/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__epoll_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__poll/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__poll </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__consume/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__consume </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__consume_buffer/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__consume_buffer </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__buffer_cnt/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__buffer_cnt </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__buffer_fd/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__buffer_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/perf_buffer__buffer/" class="md-nav__link"><span class="md-ellipsis"> perf_buffer__buffer </span></a>

<span class="md-ellipsis"> Program line info functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Program line info functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_linfo__free/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_linfo__free </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_linfo__new/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_linfo__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_linfo__lfind_addr_func/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_linfo__lfind_addr_func </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_linfo__lfind/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_linfo__lfind </span></a>

<span class="md-ellipsis"> Linker functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Linker functions

- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__new/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__new_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__new_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__add_file/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__add_file </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__add_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__add_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__add_buf/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__add_buf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__finalize/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__finalize </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_linker__free/" class="md-nav__link"><span class="md-ellipsis"> bpf_linker__free </span></a>

<span class="md-ellipsis"> Misc libbpf functions </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Misc libbpf functions

- <a href="../../../ebpf-library/libbpf/userspace/libbpf_major_version/" class="md-nav__link"><span class="md-ellipsis"> libbpf_major_version </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_minor_version/" class="md-nav__link"><span class="md-ellipsis"> libbpf_minor_version </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_version_string/" class="md-nav__link"><span class="md-ellipsis"> libbpf_version_string </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_strerror/" class="md-nav__link"><span class="md-ellipsis"> libbpf_strerror </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_bpf_attach_type_str/" class="md-nav__link"><span class="md-ellipsis"> libbpf_bpf_attach_type_str </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_bpf_link_type_str/" class="md-nav__link"><span class="md-ellipsis"> libbpf_bpf_link_type_str </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_bpf_map_type_str/" class="md-nav__link"><span class="md-ellipsis"> libbpf_bpf_map_type_str </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_bpf_prog_type_str/" class="md-nav__link"><span class="md-ellipsis"> libbpf_bpf_prog_type_str </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_set_print/" class="md-nav__link"><span class="md-ellipsis"> libbpf_set_print </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_prog_type_by_name/" class="md-nav__link"><span class="md-ellipsis"> libbpf_prog_type_by_name </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_attach_type_by_name/" class="md-nav__link"><span class="md-ellipsis"> libbpf_attach_type_by_name </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_find_vmlinux_btf_id/" class="md-nav__link"><span class="md-ellipsis"> libbpf_find_vmlinux_btf_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_probe_bpf_prog_type/" class="md-nav__link"><span class="md-ellipsis"> libbpf_probe_bpf_prog_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_probe_bpf_map_type/" class="md-nav__link"><span class="md-ellipsis"> libbpf_probe_bpf_map_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_probe_bpf_helper/" class="md-nav__link"><span class="md-ellipsis"> libbpf_probe_bpf_helper </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_num_possible_cpus/" class="md-nav__link"><span class="md-ellipsis"> libbpf_num_possible_cpus </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_register_prog_handler/" class="md-nav__link"><span class="md-ellipsis"> libbpf_register_prog_handler </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_unregister_prog_handler/" class="md-nav__link"><span class="md-ellipsis"> libbpf_unregister_prog_handler </span></a>

<span class="md-ellipsis"> Legacy APIs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Legacy APIs

- <a href="../../../ebpf-library/libbpf/userspace/libbpf_set_strict_mode/" class="md-nav__link"><span class="md-ellipsis"> libbpf_set_strict_mode </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_get_error/" class="md-nav__link"><span class="md-ellipsis"> libbpf_get_error </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/libbpf_find_kernel_btf/" class="md-nav__link"><span class="md-ellipsis"> libbpf_find_kernel_btf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__get_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__get_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_program__get_expected_attach_type/" class="md-nav__link"><span class="md-ellipsis"> bpf_program__get_expected_attach_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map__get_pin_path/" class="md-nav__link"><span class="md-ellipsis"> bpf_map__get_pin_path </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__get_raw_data/" class="md-nav__link"><span class="md-ellipsis"> btf__get_raw_data </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_ext__get_raw_data/" class="md-nav__link"><span class="md-ellipsis"> btf_ext__get_raw_data </span></a>

<span class="md-ellipsis"> Types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Types

- <a href="../../../ebpf-library/libbpf/userspace/struct-libbpf_prog_handler_opts/" class="md-nav__link"><span class="md-ellipsis"> struct libbpf_prog_handler_opts </span></a>

<span class="md-ellipsis"> BTF </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BTF

- <a href="../../../ebpf-library/libbpf/userspace/btf__free/" class="md-nav__link"><span class="md-ellipsis"> btf__free </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__new/" class="md-nav__link"><span class="md-ellipsis"> btf__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__new_split/" class="md-nav__link"><span class="md-ellipsis"> btf__new_split </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__new_empty/" class="md-nav__link"><span class="md-ellipsis"> btf__new_empty </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__new_empty_split/" class="md-nav__link"><span class="md-ellipsis"> btf__new_empty_split </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__distill_base/" class="md-nav__link"><span class="md-ellipsis"> btf__distill_base </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__parse/" class="md-nav__link"><span class="md-ellipsis"> btf__parse </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__parse_split/" class="md-nav__link"><span class="md-ellipsis"> btf__parse_split </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__parse_elf/" class="md-nav__link"><span class="md-ellipsis"> btf__parse_elf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__parse_elf_split/" class="md-nav__link"><span class="md-ellipsis"> btf__parse_elf_split </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__parse_raw/" class="md-nav__link"><span class="md-ellipsis"> btf__parse_raw </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__parse_raw_split/" class="md-nav__link"><span class="md-ellipsis"> btf__parse_raw_split </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__load_vmlinux_btf/" class="md-nav__link"><span class="md-ellipsis"> btf__load_vmlinux_btf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__load_module_btf/" class="md-nav__link"><span class="md-ellipsis"> btf__load_module_btf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__load_from_kernel_by_id/" class="md-nav__link"><span class="md-ellipsis"> btf__load_from_kernel_by_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__load_from_kernel_by_id_split/" class="md-nav__link"><span class="md-ellipsis"> btf__load_from_kernel_by_id_split </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__load_into_kernel/" class="md-nav__link"><span class="md-ellipsis"> btf__load_into_kernel </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__find_by_name/" class="md-nav__link"><span class="md-ellipsis"> btf__find_by_name </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__find_by_name_kind/" class="md-nav__link"><span class="md-ellipsis"> btf__find_by_name_kind </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__type_cnt/" class="md-nav__link"><span class="md-ellipsis"> btf__type_cnt </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__base_btf/" class="md-nav__link"><span class="md-ellipsis"> btf__base_btf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__type_by_id/" class="md-nav__link"><span class="md-ellipsis"> btf__type_by_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__pointer_size/" class="md-nav__link"><span class="md-ellipsis"> btf__pointer_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__set_pointer_size/" class="md-nav__link"><span class="md-ellipsis"> btf__set_pointer_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__endianness/" class="md-nav__link"><span class="md-ellipsis"> btf__endianness </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__set_endianness/" class="md-nav__link"><span class="md-ellipsis"> btf__set_endianness </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__resolve_size/" class="md-nav__link"><span class="md-ellipsis"> btf__resolve_size </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__resolve_type/" class="md-nav__link"><span class="md-ellipsis"> btf__resolve_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__align_of/" class="md-nav__link"><span class="md-ellipsis"> btf__align_of </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__fd/" class="md-nav__link"><span class="md-ellipsis"> btf__fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__set_fd/" class="md-nav__link"><span class="md-ellipsis"> btf__set_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__raw_data/" class="md-nav__link"><span class="md-ellipsis"> btf__raw_data </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__name_by_offset/" class="md-nav__link"><span class="md-ellipsis"> btf__name_by_offset </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__str_by_offset/" class="md-nav__link"><span class="md-ellipsis"> btf__str_by_offset </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_ext__new/" class="md-nav__link"><span class="md-ellipsis"> btf_ext__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_ext__free/" class="md-nav__link"><span class="md-ellipsis"> btf_ext__free </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_ext__raw_data/" class="md-nav__link"><span class="md-ellipsis"> btf_ext__raw_data </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_ext__endianness/" class="md-nav__link"><span class="md-ellipsis"> btf_ext__endianness </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_ext__set_endianness/" class="md-nav__link"><span class="md-ellipsis"> btf_ext__set_endianness </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__find_str/" class="md-nav__link"><span class="md-ellipsis"> btf__find_str </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_str/" class="md-nav__link"><span class="md-ellipsis"> btf__add_str </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_type/" class="md-nav__link"><span class="md-ellipsis"> btf__add_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_btf/" class="md-nav__link"><span class="md-ellipsis"> btf__add_btf </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_int/" class="md-nav__link"><span class="md-ellipsis"> btf__add_int </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_float/" class="md-nav__link"><span class="md-ellipsis"> btf__add_float </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_ptr/" class="md-nav__link"><span class="md-ellipsis"> btf__add_ptr </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_array/" class="md-nav__link"><span class="md-ellipsis"> btf__add_array </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_struct/" class="md-nav__link"><span class="md-ellipsis"> btf__add_struct </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_union/" class="md-nav__link"><span class="md-ellipsis"> btf__add_union </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_field/" class="md-nav__link"><span class="md-ellipsis"> btf__add_field </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_enum/" class="md-nav__link"><span class="md-ellipsis"> btf__add_enum </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_enum_value/" class="md-nav__link"><span class="md-ellipsis"> btf__add_enum_value </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_enum64/" class="md-nav__link"><span class="md-ellipsis"> btf__add_enum64 </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_enum64_value/" class="md-nav__link"><span class="md-ellipsis"> btf__add_enum64_value </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_fwd/" class="md-nav__link"><span class="md-ellipsis"> btf__add_fwd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_typedef/" class="md-nav__link"><span class="md-ellipsis"> btf__add_typedef </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_volatile/" class="md-nav__link"><span class="md-ellipsis"> btf__add_volatile </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_const/" class="md-nav__link"><span class="md-ellipsis"> btf__add_const </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_restrict/" class="md-nav__link"><span class="md-ellipsis"> btf__add_restrict </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_type_tag/" class="md-nav__link"><span class="md-ellipsis"> btf__add_type_tag </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_type_attr/" class="md-nav__link"><span class="md-ellipsis"> btf__add_type_attr </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_func/" class="md-nav__link"><span class="md-ellipsis"> btf__add_func </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_func_proto/" class="md-nav__link"><span class="md-ellipsis"> btf__add_func_proto </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_func_param/" class="md-nav__link"><span class="md-ellipsis"> btf__add_func_param </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_var/" class="md-nav__link"><span class="md-ellipsis"> btf__add_var </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_datasec/" class="md-nav__link"><span class="md-ellipsis"> btf__add_datasec </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_datasec_var_info/" class="md-nav__link"><span class="md-ellipsis"> btf__add_datasec_var_info </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_decl_tag/" class="md-nav__link"><span class="md-ellipsis"> btf__add_decl_tag </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__add_decl_attr/" class="md-nav__link"><span class="md-ellipsis"> btf__add_decl_attr </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__dedup/" class="md-nav__link"><span class="md-ellipsis"> btf__dedup </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf__relocate/" class="md-nav__link"><span class="md-ellipsis"> btf__relocate </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_dump__new/" class="md-nav__link"><span class="md-ellipsis"> btf_dump__new </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_dump__free/" class="md-nav__link"><span class="md-ellipsis"> btf_dump__free </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_dump__dump_type/" class="md-nav__link"><span class="md-ellipsis"> btf_dump__dump_type </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_dump__emit_type_decl/" class="md-nav__link"><span class="md-ellipsis"> btf_dump__emit_type_decl </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/btf_dump__dump_type_data/" class="md-nav__link"><span class="md-ellipsis"> btf_dump__dump_type_data </span></a>

<span class="md-ellipsis"> Low level APIs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Low level APIs

- <a href="../../../ebpf-library/libbpf/userspace/libbpf_set_memlock_rlim/" class="md-nav__link"><span class="md-ellipsis"> libbpf_set_memlock_rlim </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_create </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_load/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_load </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_btf_load/" class="md-nav__link"><span class="md-ellipsis"> bpf_btf_load </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_update_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_update_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_lookup_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_lookup_elem_flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_elem_flags </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_lookup_and_delete_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_and_delete_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_lookup_and_delete_elem_flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_and_delete_elem_flags </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_delete_elem/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_delete_elem </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_delete_elem_flags/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_delete_elem_flags </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_get_next_key/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_get_next_key </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_freeze/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_freeze </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_delete_batch/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_delete_batch </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_lookup_batch/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_batch </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_lookup_and_delete_batch/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_lookup_and_delete_batch </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_update_batch/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_update_batch </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_obj_pin/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_pin </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_obj_pin_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_pin_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_obj_get/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_get </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_obj_get_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_get_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_attach/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_attach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_detach/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_detach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_detach2/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_detach2 </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_attach_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_attach_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_detach_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_detach_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_create </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_detach/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_detach </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_update/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_update </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_iter_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_iter_create </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_get_next_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_get_next_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_get_next_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_get_next_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_btf_get_next_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_btf_get_next_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_get_next_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_get_next_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_get_fd_by_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_get_fd_by_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_get_fd_by_id_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_get_fd_by_id_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_get_fd_by_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_get_fd_by_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_get_fd_by_id_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_get_fd_by_id_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_btf_get_fd_by_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_btf_get_fd_by_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_btf_get_fd_by_id_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_btf_get_fd_by_id_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_get_fd_by_id/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_get_fd_by_id </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_get_fd_by_id_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_get_fd_by_id_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_obj_get_info_by_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_get_info_by_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_get_info_by_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_get_info_by_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_map_get_info_by_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_map_get_info_by_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_btf_get_info_by_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_btf_get_info_by_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_link_get_info_by_fd/" class="md-nav__link"><span class="md-ellipsis"> bpf_link_get_info_by_fd </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_query_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_query_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_query/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_query </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_raw_tracepoint_open_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_raw_tracepoint_open_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_raw_tracepoint_open/" class="md-nav__link"><span class="md-ellipsis"> bpf_raw_tracepoint_open </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_task_fd_query/" class="md-nav__link"><span class="md-ellipsis"> bpf_task_fd_query </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_enable_stats/" class="md-nav__link"><span class="md-ellipsis"> bpf_enable_stats </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_bind_map/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_bind_map </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_prog_test_run_opts/" class="md-nav__link"><span class="md-ellipsis"> bpf_prog_test_run_opts </span></a>
- <a href="../../../ebpf-library/libbpf/userspace/bpf_token_create/" class="md-nav__link"><span class="md-ellipsis"> bpf_token_create </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../../ebpf-library/libbpf/ebpf/" class="md-nav__link"><span class="md-ellipsis"> eBPF side </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> eBPF side

<span class="md-ellipsis"> BTF map macros / types </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> BTF map macros / types

- <a href="../../../ebpf-library/libbpf/ebpf/__uint/" class="md-nav__link"><span class="md-ellipsis"> __uint </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__type/" class="md-nav__link"><span class="md-ellipsis"> __type </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__array/" class="md-nav__link"><span class="md-ellipsis"> __array </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__ulong/" class="md-nav__link"><span class="md-ellipsis"> __ulong </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/enum-libbpf_pin_type/" class="md-nav__link"><span class="md-ellipsis"> enum libbpf_pin_type </span></a>

<span class="md-ellipsis"> Attributes </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Attributes

- <a href="../../../ebpf-library/libbpf/ebpf/__always_inline/" class="md-nav__link"><span class="md-ellipsis"> __always_inline </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__noinline/" class="md-nav__link"><span class="md-ellipsis"> __noinline </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__weak/" class="md-nav__link"><span class="md-ellipsis"> __weak </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__hidden/" class="md-nav__link"><span class="md-ellipsis"> __hidden </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__kconfig/" class="md-nav__link"><span class="md-ellipsis"> __kconfig </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__ksym/" class="md-nav__link"><span class="md-ellipsis"> __ksym </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__kptr_untrusted/" class="md-nav__link"><span class="md-ellipsis"> __kptr_untrusted </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__kptr/" class="md-nav__link"><span class="md-ellipsis"> __kptr </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__percpu_kptr/" class="md-nav__link"><span class="md-ellipsis"> __percpu_kptr </span></a>

<span class="md-ellipsis"> Global function attributes </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Global function attributes

- <a href="../../../ebpf-library/libbpf/ebpf/__arg_ctx/" class="md-nav__link"><span class="md-ellipsis"> __arg_ctx </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__arg_nonnull/" class="md-nav__link"><span class="md-ellipsis"> __arg_nonnull </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__arg_nullable/" class="md-nav__link"><span class="md-ellipsis"> __arg_nullable </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__arg_trusted/" class="md-nav__link"><span class="md-ellipsis"> __arg_trusted </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/__arg_arena/" class="md-nav__link"><span class="md-ellipsis"> __arg_arena </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/SEC/" class="md-nav__link"><span class="md-ellipsis"> SEC </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/KERNEL_VERSION/" class="md-nav__link"><span class="md-ellipsis"> KERNEL_VERSION </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/offsetof/" class="md-nav__link"><span class="md-ellipsis"> offsetof </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/container_of/" class="md-nav__link"><span class="md-ellipsis"> container_of </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/barrier/" class="md-nav__link"><span class="md-ellipsis"> barrier </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/barrier_var/" class="md-nav__link"><span class="md-ellipsis"> barrier_var </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/__bpf_unreachable/" class="md-nav__link"><span class="md-ellipsis"> __bpf_unreachable </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_tail_call_static/" class="md-nav__link"><span class="md-ellipsis"> bpf_tail_call_static </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_ksym_exists/" class="md-nav__link"><span class="md-ellipsis"> bpf_ksym_exists </span></a>

<span class="md-ellipsis"> Printf macros </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Printf macros

- <a href="../../../ebpf-library/libbpf/ebpf/bpf_seq_printf/" class="md-nav__link"><span class="md-ellipsis"> BPF_SEQ_PRINTF </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_snprintf/" class="md-nav__link"><span class="md-ellipsis"> BPF_SNPRINTF </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_printk/" class="md-nav__link"><span class="md-ellipsis"> bpf_printk </span></a>

<span class="md-ellipsis"> Open coded iterator loop macros </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Open coded iterator loop macros

- <a href="../../../ebpf-library/libbpf/ebpf/bpf_for_each/" class="md-nav__link"><span class="md-ellipsis"> bpf_for_each </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_for/" class="md-nav__link"><span class="md-ellipsis"> bpf_for </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_repeat/" class="md-nav__link"><span class="md-ellipsis"> bpf_repeat </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_htons/" class="md-nav__link"><span class="md-ellipsis"> bpf_htons </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_ntohs/" class="md-nav__link"><span class="md-ellipsis"> bpf_ntohs </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_htonl/" class="md-nav__link"><span class="md-ellipsis"> bpf_htonl </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_ntohl/" class="md-nav__link"><span class="md-ellipsis"> bpf_ntohl </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_cpu_to_be64/" class="md-nav__link"><span class="md-ellipsis"> bpf_cpu_to_be64 </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_be64_to_cpu/" class="md-nav__link"><span class="md-ellipsis"> bpf_be64_to_cpu </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_PARM/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_PARM </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_PARM_SYSCALL/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_PARM_SYSCALL </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_RET/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_RET </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_FP/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_FP </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_RC/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_RC </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_SP/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_SP </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_IP/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_IP </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/PT_REGS_SYSCALL_REGS/" class="md-nav__link"><span class="md-ellipsis"> PT_REGS_SYSCALL_REGS </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_PROG/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_PROG2/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROG2 </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_KPROBE/" class="md-nav__link"><span class="md-ellipsis"> BPF_KPROBE </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_UPROBE/" class="md-nav__link"><span class="md-ellipsis"> BPF_UPROBE </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_KRETPROBE/" class="md-nav__link"><span class="md-ellipsis"> BPF_KRETPROBE </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_URETPROBE/" class="md-nav__link"><span class="md-ellipsis"> BPF_URETPROBE </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_KSYSCALL/" class="md-nav__link"><span class="md-ellipsis"> BPF_KSYSCALL </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/BPF_KPROBE_SYSCALL/" class="md-nav__link"><span class="md-ellipsis"> BPF_KPROBE_SYSCALL </span></a>

<span class="md-ellipsis"> CO-RE memory access </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CO-RE memory access

- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_read/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_read </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_STR_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_STR_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_read_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_read_str </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_USER/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_USER </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_USER_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_USER_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_read_user/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_read_user </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_USER_STR_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_USER_STR_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_read_user_str/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_read_user_str </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_BITFIELD/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_BITFIELD </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_READ_BITFIELD_PROBED/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_READ_BITFIELD_PROBED </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_CORE_WRITE_BITFIELD/" class="md-nav__link"><span class="md-ellipsis"> BPF_CORE_WRITE_BITFIELD </span></a>

<span class="md-ellipsis"> CO-RE queries </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> CO-RE queries

- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_field_exists/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_field_exists </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_field_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_field_size </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_field_offset/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_field_offset </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_type_id_local/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_type_id_local </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_type_id_kernel/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_type_id_kernel </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_type_exists/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_type_exists </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_type_matches/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_type_matches </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_type_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_type_size </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_enum_value_exists/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_enum_value_exists </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_core_enum_value/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_enum_value </span></a>

<a href="../../../ebpf-library/libbpf/ebpf/bpf_core_cast/" class="md-nav__link"><span class="md-ellipsis"> bpf_core_cast </span></a>

<span class="md-ellipsis"> Non CO-RE macros </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Non CO-RE macros

- <a href="../../../ebpf-library/libbpf/ebpf/BPF_PROBE_READ/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROBE_READ </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_PROBE_READ_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROBE_READ_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_PROBE_READ_USER_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROBE_READ_USER_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_PROBE_READ_STR_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROBE_READ_STR_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_PROBE_READ_USER_STR_INTO/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROBE_READ_USER_STR_INTO </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/BPF_PROBE_READ_USER/" class="md-nav__link"><span class="md-ellipsis"> BPF_PROBE_READ_USER </span></a>

<span class="md-ellipsis"> Utility macros </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Utility macros

- <a href="../../../ebpf-library/libbpf/ebpf/___bpf_fill/" class="md-nav__link"><span class="md-ellipsis"> ___bpf_fill </span></a>

<span class="md-ellipsis"> USDT macros </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> USDT macros

- <a href="../../../ebpf-library/libbpf/ebpf/BPF_USDT/" class="md-nav__link"><span class="md-ellipsis"> BPF_USDT </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_usdt_arg_cnt/" class="md-nav__link"><span class="md-ellipsis"> bpf_usdt_arg_cnt </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_usdt_arg_size/" class="md-nav__link"><span class="md-ellipsis"> bpf_usdt_arg_size </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_usdt_arg/" class="md-nav__link"><span class="md-ellipsis"> bpf_usdt_arg </span></a>
- <a href="../../../ebpf-library/libbpf/ebpf/bpf_usdt_cookie/" class="md-nav__link"><span class="md-ellipsis"> bpf_usdt_cookie </span></a>

<a href="../../../ebpf-library/libbpf/concepts/" class="md-nav__link"><span class="md-ellipsis"> Concepts </span></a>

<span class="md-ellipsis"> Libxdp </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Libxdp

<a href="../../../ebpf-library/libxdp/libxdp/" class="md-nav__link"><span class="md-ellipsis"> Concept </span></a>

<span class="md-ellipsis"> Manage programs </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Manage programs

<span class="md-ellipsis"> Load </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Load

- <a href="../../../ebpf-library/libxdp/functions/xdp_program__from_bpf_obj/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__from_bpf_obj </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__find_file/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__find_file </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__open_file/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__open_file </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__from_fd/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__from_fd </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__from_id/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__from_id </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__from_pin/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__from_pin </span></a>

<span class="md-ellipsis"> Metadata </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Metadata

- <a href="../../../ebpf-library/libxdp/functions/xdp_program__run_prio/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__run_prio </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__set_run_prio/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__set_run_prio </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__chain_call_enabled/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__chain_call_enabled </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__set_chain_call_enabled/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__set_chain_call_enabled </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_program__print_chain_call_actions/" class="md-nav__link"><span class="md-ellipsis"> xdp_program__print_chain_call_actions </span></a>

<span class="md-ellipsis"> Dispatcher </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Dispatcher

- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__get_from_ifindex/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__get_from_ifindex </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__next_prog/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__next_prog </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__close/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__close </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__detach/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__detach </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__attach_mode/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__attach_mode </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__main_prog/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__main_prog </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__hw_prog/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__hw_prog </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xdp_multiprog__is_legacy/" class="md-nav__link"><span class="md-ellipsis"> xdp_multiprog__is_legacy </span></a>

<span class="md-ellipsis"> AF_XDP sockets </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> AF_XDP sockets

<span class="md-ellipsis"> Control path </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Control path

<span class="md-ellipsis"> Umem Area </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Umem Area

- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__create/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__create </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__create_with_fd/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__create_with_fd </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__delete/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__delete </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__fd/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__fd </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__get_data/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__get_data </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__extract_addr/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__extract_addr </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__extract_offset/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__extract_offset </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_umem__add_offset_to_addr/" class="md-nav__link"><span class="md-ellipsis"> xsk_umem__add_offset_to_addr </span></a>

<span class="md-ellipsis"> Sockets </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Sockets

- <a href="../../../ebpf-library/libxdp/functions/xsk_socket__create/" class="md-nav__link"><span class="md-ellipsis"> xsk_socket__create </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_socket__create_shared/" class="md-nav__link"><span class="md-ellipsis"> xsk_socket__create_shared </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_socket__delete/" class="md-nav__link"><span class="md-ellipsis"> xsk_socket__delete </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_socket__fd/" class="md-nav__link"><span class="md-ellipsis"> xsk_socket__fd </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_setup_xdp_prog/" class="md-nav__link"><span class="md-ellipsis"> xsk_setup_xdp_prog </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_socket__update_xskmap/" class="md-nav__link"><span class="md-ellipsis"> xsk_socket__update_xskmap </span></a>

<span class="md-ellipsis"> Data path </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Data path

<span class="md-ellipsis"> Producer rings </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Producer rings

- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_prod__reserve/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_prod__reserve </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_prod__submit/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_prod__submit </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_prod__fill_addr/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_prod__fill_addr </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_prod__tx_desc/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_prod__tx_desc </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_prod__needs_wakeup/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_prod__needs_wakeup </span></a>

<span class="md-ellipsis"> Consumer rings </span> <span class="md-nav__icon md-icon"></span>

<span class="md-nav__icon md-icon"></span> Consumer rings

- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_cons__peek/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_cons__peek </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_cons__cancel/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_cons__cancel </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_cons__release/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_cons__release </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_cons__comp_addr/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_cons__comp_addr </span></a>
- <a href="../../../ebpf-library/libxdp/functions/xsk_ring_cons__rx_desc/" class="md-nav__link"><span class="md-ellipsis"> xsk_ring_cons__rx_desc </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../../ebpf-library/scx/" class="md-nav__link"><span class="md-ellipsis"> SCX Common </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> SCX Common

- <a href="../../../ebpf-library/scx/BPF_FOR_EACH_ITER/" class="md-nav__link"><span class="md-ellipsis"> BPF_FOR_EACH_ITER </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_bstr_preamble/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_bstr_preamble </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_exit/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_exit </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_error/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_error </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_dump/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dump </span></a>
- <a href="../../../ebpf-library/scx/BPF_STRUCT_OPS/" class="md-nav__link"><span class="md-ellipsis"> BPF_STRUCT_OPS </span></a>
- <a href="../../../ebpf-library/scx/BPF_STRUCT_OPS_SLEEPABLE/" class="md-nav__link"><span class="md-ellipsis"> BPF_STRUCT_OPS_SLEEPABLE </span></a>
- <a href="../../../ebpf-library/scx/RESIZABLE_ARRAY/" class="md-nav__link"><span class="md-ellipsis"> RESIZABLE_ARRAY </span></a>
- <a href="../../../ebpf-library/scx/ARRAY_ELEM_PTR/" class="md-nav__link"><span class="md-ellipsis"> ARRAY_ELEM_PTR </span></a>
- <a href="../../../ebpf-library/scx/MEMBER_VPTR/" class="md-nav__link"><span class="md-ellipsis"> MEMBER_VPTR </span></a>
- <a href="../../../ebpf-library/scx/__contains/" class="md-nav__link"><span class="md-ellipsis"> __contains </span></a>
- <a href="../../../ebpf-library/scx/private/" class="md-nav__link"><span class="md-ellipsis"> private </span></a>
- <a href="../../../ebpf-library/scx/bpf_obj_new/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_new </span></a>
- <a href="../../../ebpf-library/scx/bpf_obj_drop/" class="md-nav__link"><span class="md-ellipsis"> bpf_obj_drop </span></a>
- <a href="../../../ebpf-library/scx/bpf_rbtree_add/" class="md-nav__link"><span class="md-ellipsis"> bpf_rbtree_add </span></a>
- <a href="../../../ebpf-library/scx/bpf_refcount_acquire/" class="md-nav__link"><span class="md-ellipsis"> bpf_refcount_acquire </span></a>
- <a href="../../../ebpf-library/scx/cast_mask/" class="md-nav__link"><span class="md-ellipsis"> cast_mask </span></a>
- <a href="../../../ebpf-library/scx/likely/" class="md-nav__link"><span class="md-ellipsis"> likely </span></a>
- <a href="../../../ebpf-library/scx/unlikely/" class="md-nav__link"><span class="md-ellipsis"> unlikely </span></a>
- <a href="../../../ebpf-library/scx/READ_ONCE/" class="md-nav__link"><span class="md-ellipsis"> READ_ONCE </span></a>
- <a href="../../../ebpf-library/scx/WRITE_ONCE/" class="md-nav__link"><span class="md-ellipsis"> WRITE_ONCE </span></a>
- <a href="../../../ebpf-library/scx/log2_u32/" class="md-nav__link"><span class="md-ellipsis"> log2_u32 </span></a>
- <a href="../../../ebpf-library/scx/log2_u64/" class="md-nav__link"><span class="md-ellipsis"> log2_u64 </span></a>
- <a href="../../../ebpf-library/scx/__COMPAT_ENUM_OR_ZERO/" class="md-nav__link"><span class="md-ellipsis"> __COMPAT_ENUM_OR_ZERO </span></a>
- <a href="../../../ebpf-library/scx/__COMPAT_scx_bpf_task_cgroup/" class="md-nav__link"><span class="md-ellipsis"> __COMPAT_scx_bpf_task_cgroup </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_dsq_insert/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_insert </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_dsq_insert_vtime/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_insert_vtime </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_dsq_move_to_local/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_dsq_move_to_local </span></a>
- <a href="../../../ebpf-library/scx/__COMPAT_scx_bpf_dsq_move_set_slice/" class="md-nav__link"><span class="md-ellipsis"> __COMPAT_scx_bpf_dsq_move_set_slice </span></a>
- <a href="../../../ebpf-library/scx/__COMPAT_scx_bpf_dsq_move_set_vtime/" class="md-nav__link"><span class="md-ellipsis"> __COMPAT_scx_bpf_dsq_move_set_vtime </span></a>
- <a href="../../../ebpf-library/scx/__COMPAT_scx_bpf_dsq_move/" class="md-nav__link"><span class="md-ellipsis"> __COMPAT_scx_bpf_dsq_move </span></a>
- <a href="../../../ebpf-library/scx/__COMPAT_scx_bpf_dsq_move_vtime/" class="md-nav__link"><span class="md-ellipsis"> __COMPAT_scx_bpf_dsq_move_vtime </span></a>
- <a href="../../../ebpf-library/scx/SCX_OPS_DEFINE/" class="md-nav__link"><span class="md-ellipsis"> SCX_OPS_DEFINE </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_reenqueue_local/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_reenqueue_local </span></a>
- <a href="../../../ebpf-library/scx/scx_bpf_select_cpu_and/" class="md-nav__link"><span class="md-ellipsis"> scx_bpf_select_cpu_and </span></a>

<div class="md-nav__link md-nav__container">

<a href="../../../concepts/" class="md-nav__link"><span class="md-ellipsis"> Concepts </span></a> <span class="md-nav__icon md-icon"></span>

</div>

<span class="md-nav__icon md-icon"></span> Concepts

- <a href="../../../concepts/core/" class="md-nav__link"><span class="md-ellipsis"> BPF CO-RE </span></a>
- <a href="../../../concepts/btf/" class="md-nav__link"><span class="md-ellipsis"> BTF </span></a>
- <a href="../../../concepts/elf/" class="md-nav__link"><span class="md-ellipsis"> ELF </span></a>

<a href="../../../meta/" class="md-nav__link"><span class="md-ellipsis"> Meta docs </span></a>

<a href="../../../faq/" class="md-nav__link"><span class="md-ellipsis"> FAQ </span></a>

</div>

</div>

</div>

<div class="md-sidebar md-sidebar--secondary" md-component="sidebar" md-type="toc">

<div class="md-sidebar__scrollwrap">

<div class="md-sidebar__inner">

<span class="md-nav__icon md-icon"></span> Table of contents

- <a href="#definition" class="md-nav__link"><span class="md-ellipsis"> Definition </span></a>
  - <a href="#returns" class="md-nav__link"><span class="md-ellipsis"> Returns </span></a>
- <a href="#usage" class="md-nav__link"><span class="md-ellipsis"> Usage </span></a>
  - <a href="#program-types" class="md-nav__link"><span class="md-ellipsis"> Program types </span></a>
  - <a href="#example" class="md-nav__link"><span class="md-ellipsis"> Example </span></a>

</div>

</div>

</div>

<div class="md-content" md-component="content">

<a href="https://github.com/isovalent/ebpf-docs/blob/master/docs/linux/helper-function/bpf_ringbuf_output.md" class="md-content__button md-icon" title="Edit this page"><img src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTIwLjcxIDcuMDRjLjM5LS4zOS4zOS0xLjA0IDAtMS40MWwtMi4zNC0yLjM0Yy0uMzctLjM5LTEuMDItLjM5LTEuNDEgMGwtMS44NCAxLjgzIDMuNzUgMy43NU0zIDE3LjI1VjIxaDMuNzVMMTcuODEgOS45M2wtMy43NS0zLjc1eiIgLz48L3N2Zz4=" /></a> <a href="https://github.com/isovalent/ebpf-docs/raw/master/docs/linux/helper-function/bpf_ringbuf_output.md" class="md-content__button md-icon" title="View source of this page"><img src="data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTEyIDlhMyAzIDAgMCAwLTMgMyAzIDMgMCAwIDAgMyAzIDMgMyAwIDAgMCAzLTMgMyAzIDAgMCAwLTMtM20wIDhhNSA1IDAgMCAxLTUtNSA1IDUgMCAwIDEgNS01IDUgNSAwIDAgMSA1IDUgNSA1IDAgMCAxLTUgNW0wLTEyLjVDNyA0LjUgMi43MyA3LjYxIDEgMTJjMS43MyA0LjM5IDYgNy41IDExIDcuNXM5LjI3LTMuMTEgMTEtNy41Yy0xLjczLTQuMzktNi03LjUtMTEtNy41IiAvPjwvc3ZnPg==" /></a>

<div class="section section1">

# Helper function `bpf_ringbuf_output`

[<span class="twemoji">![](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTcuNzUgNi41YTEuMjUgMS4yNSAwIDEgMCAwIDIuNSAxLjI1IDEuMjUgMCAwIDAgMC0yLjUiIC8+PHBhdGggZD0iTTIuNSAxaDguNDRhMS41IDEuNSAwIDAgMSAxLjA2LjQ0bDEwLjI1IDEwLjI1YTEuNSAxLjUgMCAwIDEgMCAyLjEybC04LjQ0IDguNDRhMS41IDEuNSAwIDAgMS0yLjEyIDBMMS40NCAxMkExLjUgMS41IDAgMCAxIDEgMTAuOTRWMi41QTEuNSAxLjUgMCAwIDEgMi41IDFtMCAxLjV2OC40NGwxMC4yNSAxMC4yNSA4LjQ0LTguNDRMMTAuOTQgMi41WiIgLz48L3N2Zz4=)</span> v5.8](https://github.com/torvalds/linux/commit/457f44363a8894135c85b7a9afd2bd8196db24ab)

<div class="section section2">

## Definition

> Copyright (c) 2015 The Libbpf Authors. All rights reserved.

Copy *size* bytes from *data* into a ring buffer *ringbuf*. If **BPF_RB_NO_WAKEUP** is specified in *flags*, no notification of new data availability is sent. If **BPF_RB_FORCE_WAKEUP** is specified in *flags*, notification of new data availability is sent unconditionally. If **0** is specified in *flags*, an adaptive notification of new data availability is sent.

An adaptive notification is a notification sent whenever the user-space process has caught up and consumed all available payloads. In case the user-space process is still processing a previous payload, then no notification is needed as it will process the newly added payload automatically.

<div class="section section3">

### Returns

0 on success, or a negative error in case of failure.

<span class="k">`static`</span><span class="w">` `</span><span class="kt">`long`</span><span class="w">` `</span><span class="p">`(`</span><span class="o">`*`</span><span class="w">` `</span><span class="k">`const`</span><span class="w">` `</span><span class="n">`bpf_ringbuf_output`</span><span class="p">`)(`</span><span class="kt">`void`</span><span class="w">` `</span><span class="o">`*`</span><span class="n">`ringbuf`</span><span class="p">`,`</span><span class="w">` `</span><span class="kt">`void`</span><span class="w">` `</span><span class="o">`*`</span><span class="n">`data`</span><span class="p">`,`</span><span class="w">` `</span><span class="n">`__u64`</span><span class="w">` `</span><span class="n">`size`</span><span class="p">`,`</span><span class="w">` `</span><span class="n">`__u64`</span><span class="w">` `</span><span class="n">`flags`</span><span class="p">`)`</span><span class="w">` `</span><span class="o">`=`</span><span class="w">` `</span><span class="p">`(`</span><span class="kt">`void`</span><span class="w">` `</span><span class="o">`*`</span><span class="p">`)`</span><span class="w">` `</span><span class="mi">`130`</span><span class="p">`;`</span>

</div>

</div>

<div class="section section2">

## Usage

The `ringbuf` must be a pointer to the ring buffer map. `data` is a pointer to the data that needs to be copied into the ring buffer. The `size` argument specifies the number of bytes to be copied. The `flags` argument defines how the notification of the new data availability should be handled.

This function incurs an extra memory copy operation in comparison to using [`bpf_ringbuf_reserve`](../bpf_ringbuf_reserve/)/[`bpf_ringbuf_submit`](../bpf_ringbuf_submit/)/[`bpf_ringbuf_discard`](../bpf_ringbuf_discard/), but allows submitting records of lengths unknown to the [verifier](https://www.kernel.org/doc/html/next/bpf/ringbuf.html).

<div class="section section3">

### Program types

This helper call can be used in the following program types:

- [`BPF_PROG_TYPE_CGROUP_DEVICE`](../../program-type/BPF_PROG_TYPE_CGROUP_DEVICE/)
- [`BPF_PROG_TYPE_CGROUP_SKB`](../../program-type/BPF_PROG_TYPE_CGROUP_SKB/)
- [`BPF_PROG_TYPE_CGROUP_SOCK`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCK/)
- [`BPF_PROG_TYPE_CGROUP_SOCKOPT`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCKOPT/)
- [`BPF_PROG_TYPE_CGROUP_SOCK_ADDR`](../../program-type/BPF_PROG_TYPE_CGROUP_SOCK_ADDR/)
- [`BPF_PROG_TYPE_CGROUP_SYSCTL`](../../program-type/BPF_PROG_TYPE_CGROUP_SYSCTL/)
- [`BPF_PROG_TYPE_FLOW_DISSECTOR`](../../program-type/BPF_PROG_TYPE_FLOW_DISSECTOR/)
- [`BPF_PROG_TYPE_KPROBE`](../../program-type/BPF_PROG_TYPE_KPROBE/)
- [`BPF_PROG_TYPE_LSM`](../../program-type/BPF_PROG_TYPE_LSM/)
- [`BPF_PROG_TYPE_LWT_IN`](../../program-type/BPF_PROG_TYPE_LWT_IN/)
- [`BPF_PROG_TYPE_LWT_OUT`](../../program-type/BPF_PROG_TYPE_LWT_OUT/)
- [`BPF_PROG_TYPE_LWT_SEG6LOCAL`](../../program-type/BPF_PROG_TYPE_LWT_SEG6LOCAL/)
- [`BPF_PROG_TYPE_LWT_XMIT`](../../program-type/BPF_PROG_TYPE_LWT_XMIT/)
- [`BPF_PROG_TYPE_NETFILTER`](../../program-type/BPF_PROG_TYPE_NETFILTER/)
- [`BPF_PROG_TYPE_PERF_EVENT`](../../program-type/BPF_PROG_TYPE_PERF_EVENT/)
- [`BPF_PROG_TYPE_RAW_TRACEPOINT`](../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT/)
- [`BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE`](../../program-type/BPF_PROG_TYPE_RAW_TRACEPOINT_WRITABLE/)
- [`BPF_PROG_TYPE_SCHED_ACT`](../../program-type/BPF_PROG_TYPE_SCHED_ACT/)
- [`BPF_PROG_TYPE_SCHED_CLS`](../../program-type/BPF_PROG_TYPE_SCHED_CLS/)
- [`BPF_PROG_TYPE_SK_LOOKUP`](../../program-type/BPF_PROG_TYPE_SK_LOOKUP/)
- [`BPF_PROG_TYPE_SK_MSG`](../../program-type/BPF_PROG_TYPE_SK_MSG/)
- [`BPF_PROG_TYPE_SK_REUSEPORT`](../../program-type/BPF_PROG_TYPE_SK_REUSEPORT/)
- [`BPF_PROG_TYPE_SK_SKB`](../../program-type/BPF_PROG_TYPE_SK_SKB/)
- [`BPF_PROG_TYPE_SOCKET_FILTER`](../../program-type/BPF_PROG_TYPE_SOCKET_FILTER/)
- [`BPF_PROG_TYPE_SOCK_OPS`](../../program-type/BPF_PROG_TYPE_SOCK_OPS/)
- [`BPF_PROG_TYPE_STRUCT_OPS`](../../program-type/BPF_PROG_TYPE_STRUCT_OPS/)
- [`BPF_PROG_TYPE_SYSCALL`](../../program-type/BPF_PROG_TYPE_SYSCALL/)
- [`BPF_PROG_TYPE_TRACEPOINT`](../../program-type/BPF_PROG_TYPE_TRACEPOINT/)
- [`BPF_PROG_TYPE_TRACING`](../../program-type/BPF_PROG_TYPE_TRACING/)
- [`BPF_PROG_TYPE_XDP`](../../program-type/BPF_PROG_TYPE_XDP/)

</div>

<div class="section section3">

### Example

<div class="highlight">

    // Copy data into the ring buffer
    bpf_ringbuf_output(&my_ringbuf, &my_data, sizeof(my_data), 0);

</div>

</div>

</div>

</div>

<span class="md-source-file__fact"> <span class="md-icon" title="Last update"> ![](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTIxIDEzLjFjLS4xIDAtLjMuMS0uNC4ybC0xIDEgMi4xIDIuMSAxLTFjLjItLjIuMi0uNiAwLS44bC0xLjMtMS4zYy0uMS0uMS0uMi0uMi0uNC0uMm0tMS45IDEuOC02LjEgNlYyM2gyLjFsNi4xLTYuMXpNMTIuNSA3djUuMmw0IDIuNC0xIDFMMTEgMTNWN3pNMTEgMjEuOWMtNS4xLS41LTktNC44LTktOS45QzIgNi41IDYuNSAyIDEyIDJjNS4zIDAgOS42IDQuMSAxMCA5LjMtLjMtLjEtLjYtLjItMS0uMnMtLjcuMS0xIC4yQzE5LjYgNy4yIDE2LjIgNCAxMiA0Yy00LjQgMC04IDMuNi04IDggMCA0LjEgMy4xIDcuNSA3LjEgNy45bC0uMS4yeiIgLz48L3N2Zz4=) </span> <span class="git-revision-date-localized-plugin git-revision-date-localized-plugin-date">August 25, 2024</span> </span> <span class="md-source-file__fact"> <span class="md-icon" title="Created"> ![](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTE0LjQ3IDE1LjA4IDExIDEzVjdoMS41djUuMjVsMy4wOCAxLjgzYy0uNDEuMjgtLjc5LjYyLTEuMTEgMW0tMS4zOSA0Ljg0Yy0uMzYuMDUtLjcxLjA4LTEuMDguMDgtNC40MiAwLTgtMy41OC04LThzMy41OC04IDgtOCA4IDMuNTggOCA4YzAgLjM3LS4wMy43Mi0uMDggMS4wOC42OS4xIDEuMzMuMzIgMS45Mi42NC4xLS41Ni4xNi0xLjEzLjE2LTEuNzIgMC01LjUtNC41LTEwLTEwLTEwUzIgNi41IDIgMTJzNC40NyAxMCAxMCAxMGMuNTkgMCAxLjE2LS4wNiAxLjcyLS4xNi0uMzItLjU5LS41NC0xLjIzLS42NC0xLjkyTTE4IDE1djNoLTN2MmgzdjNoMnYtM2gzdi0yaC0zdi0zeiIgLz48L3N2Zz4=) </span> <span class="git-revision-date-localized-plugin git-revision-date-localized-plugin-date">January 25, 2023</span> </span> <span class="md-source-file__fact"> <span class="md-icon" title="Contributors"> ![](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTEyIDJBMTAgMTAgMCAwIDAgMiAxMmMwIDQuNDIgMi44NyA4LjE3IDYuODQgOS41LjUuMDguNjYtLjIzLjY2LS41di0xLjY5Yy0yLjc3LjYtMy4zNi0xLjM0LTMuMzYtMS4zNC0uNDYtMS4xNi0xLjExLTEuNDctMS4xMS0xLjQ3LS45MS0uNjIuMDctLjYuMDctLjYgMSAuMDcgMS41MyAxLjAzIDEuNTMgMS4wMy44NyAxLjUyIDIuMzQgMS4wNyAyLjkxLjgzLjA5LS42NS4zNS0xLjA5LjYzLTEuMzQtMi4yMi0uMjUtNC41NS0xLjExLTQuNTUtNC45MiAwLTEuMTEuMzgtMiAxLjAzLTIuNzEtLjEtLjI1LS40NS0xLjI5LjEtMi42NCAwIDAgLjg0LS4yNyAyLjc1IDEuMDIuNzktLjIyIDEuNjUtLjMzIDIuNS0uMzNzMS43MS4xMSAyLjUuMzNjMS45MS0xLjI5IDIuNzUtMS4wMiAyLjc1LTEuMDIuNTUgMS4zNS4yIDIuMzkuMSAyLjY0LjY1LjcxIDEuMDMgMS42IDEuMDMgMi43MSAwIDMuODItMi4zNCA0LjY2LTQuNTcgNC45MS4zNi4zMS42OS45Mi42OSAxLjg1VjIxYzAgLjI3LjE2LjU5LjY3LjVDMTkuMTQgMjAuMTYgMjIgMTYuNDIgMjIgMTJBMTAgMTAgMCAwIDAgMTIgMiIgLz48L3N2Zz4=) </span> GitHub </span>

<a href="https://github.com/dylandreimerink" class="md-author" title="@dylandreimerink"><img src="https://avatars.githubusercontent.com/u/1799415?v=4&amp;size=72" alt="dylandreimerink" /></a> <a href="https://github.com/rutu-sh" class="md-author" title="@rutu-sh"><img src="https://avatars.githubusercontent.com/u/28715344?v=4&amp;size=72" alt="rutu-sh" /></a> <a href="https://github.com/parttimenerd" class="md-author" title="@parttimenerd"><img src="https://avatars.githubusercontent.com/u/490655?v=4&amp;size=72" alt="parttimenerd" /></a> <a href="https://github.com/dkanaliev" class="md-author" title="@dkanaliev"><img src="https://avatars.githubusercontent.com/u/19514094?v=4&amp;size=72" alt="dkanaliev" /></a>

</div>

</div>

![](data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdib3g9IjAgMCAyNCAyNCI+PHBhdGggZD0iTTEzIDIwaC0yVjhsLTUuNSA1LjUtMS40Mi0xLjQyTDEyIDQuMTZsNy45MiA3LjkyLTEuNDIgMS40MkwxMyA4eiIgLz48L3N2Zz4=) Back to top

</div>
