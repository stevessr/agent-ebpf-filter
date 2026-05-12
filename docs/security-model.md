# Security Model

## Current privilege split

Today the backend still combines:

- eBPF bootstrap / map access
- HTTP + WebSocket APIs
- UI/static serving
- local UDS wrapper control
- replay / export / storage

That means the current security posture depends heavily on:

- release-mode API authentication
- dangerous-feature runtime gating
- UDS peer-credential checks
- least-privilege child command execution

## Control-plane protections already present

- release-mode token auth for `/config/**`, `/system/**`, `/ws*`, `/metrics`, `/events/*`, `/sandbox/**`, `/register`, `/unregister`, and shell-session APIs
- per-hook secret support for `/hooks/event`
- PTY, `/system/run`, hook installation, and policy mutation disabled by default until explicitly enabled
- `/tmp/agent-ebpf.sock` created with `0600`
- peer credential verification on the UDS wrapper socket
- OS-level cgroup/LSM policy maps pinned with restrictive `0600` permissions
  and mutated through authenticated policy APIs rather than direct
  unprivileged map writes

## Kernel-enforced policy paths

The backend currently owns two explicit OS-level enforcement paths:

- cgroup/connect and cgroup/sendmsg blocking for exact cgroup ids, IPv4/IPv6 destinations, and
  TCP/UDP destination ports. The decision happens in cgroup/connect4,
  cgroup/connect6, cgroup/sendmsg4, and cgroup/sendmsg6 hooks before the
  TCP/UDP connect, existing connected UDP send, or unconnected UDP sendmsg
  completes. IPv4 block entries are also honored for IPv4-mapped IPv6 socket
  destinations.
- BPF LSM blocking for executable paths, executable basenames, and file or
  directory basenames. The decision happens in `bprm_check_security`,
  `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`,
  `inode_mkdir`, `inode_rmdir`, `inode_mknod`, and `inode_rename`, returning
  `EACCES` for matches, including existing-fd `ftruncate` / `fchmod`
  operations that flow through `inode_setattr`.

Fresh boots start with empty OS-enforcement maps unless an earlier privileged
run left pinned policy entries. These kernel decisions are deterministic map
lookups; wrapper, hook, ML, or LLM policy can suggest entries but is not in the
synchronous cgroup/LSM decision loop.

## Data-plane trust assumptions

- kernel/eBPF events are treated as the factual execution source
- wrapper/native hook events are semantic declarations
- the system raises alerts when the semantic layer and factual layer diverge

## Planned hardening direction

The roadmap target is to split into:

- `agent-ebpf-privileged`
  - load / attach eBPF
  - manage pinned maps and links
  - read ringbuf
  - own cgroup / LSM policy-map mutation and attach lifecycle
- `agent-ebpf-server`
  - HTTP / WS / UI
  - storage / replay
  - graph / export / OTLP / MCP

That split is **not fully implemented yet**. This document should stay honest about that boundary.

## Operational guidance

- treat the app as a local workstation tool unless reverse-proxy auth and host hardening are also in place
- rotate runtime tokens if you enable release-mode remote access
- keep hook secrets per CLI rather than reusing one global secret everywhere
- enable PTY or `/system/run` only when you explicitly need them
