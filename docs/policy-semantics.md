# Policy Semantics

This document describes what policy matching means **in the current repository state**.

## Current policy surfaces

### 1. Wrapper rules

Configured under `/config/rules` and applied to `agent-wrapper` requests.

Supported actions:

- `ALLOW`
- `BLOCK`
- `ALERT`
- `REWRITE`

Rules are keyed by exact `comm` and can optionally include a regex + replacement or explicit rewritten args.

### 2. Tracked commands

`tracked_comms` is an exact 16-byte command-name match.

Examples:

- `git`
- `python`
- `node`
- `npm`

### 3. Tracked paths

`tracked_paths` is an exact 256-byte path match.

Examples:

- `/workspace/repo/.env`
- `/home/steve/.ssh/id_rsa`

### 4. Tracked prefixes

The repo now also has tracked prefixes in userspace/config, but the exact-path model is still the most important truth when describing what the BPF program itself matches cheaply.

### 5. OS-level cgroup/connect + sendmsg policy

Configured through `/sandbox/cgroup/*` and stored in pinned eBPF maps under
`/sys/fs/bpf/agent-ebpf/cgroup_sandbox`.

Supported exact-match keys:

- cgroup v2 inode id, including ids resolved from a PID's current cgroup
- IPv4 destination address, including IPv4-mapped IPv6 socket destinations
- IPv6 destination address
- TCP/UDP destination port

The cgroup/connect4, cgroup/connect6, cgroup/sendmsg4, and cgroup/sendmsg6 hooks return a kernel deny for matching
outbound TCP connects, UDP connected-socket connects, existing connected UDP sends, and unconnected UDP sendto/sendmsg.
IPv4 block entries also deny IPv4-mapped IPv6 destinations such as `::ffff:a.b.c.d`;
API inputs in that form normalize to the equivalent IPv4 block key.
This is not CIDR, domain, recursive process-tree, or policy-tree matching.
Existing TCP streams established before a matching block is added are not
retroactively terminated by these cgroup hooks.

### 6. OS-level BPF LSM policy

Configured through `/sandbox/lsm/*` and stored in pinned eBPF maps under
`/sys/fs/bpf/agent-ebpf/lsm_enforcer`.

Supported exact-match keys:

- executable path for `bprm_check_security`
- executable basename for `bprm_check_security`
- file or directory basename for `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`, `inode_link`,
  `inode_symlink`, `inode_unlink`, `inode_mkdir`, `inode_rmdir`, `inode_mknod`,
  and `inode_rename`

Matching LSM decisions return `EACCES` before the target operation completes.
Existing writable mappings established before a basename is blocked cannot be
retroactively revoked, but new `mmap_file` mappings and later `file_mprotect`
permission changes are denied. Existing-fd `ftruncate` / `fchmod`-style
metadata changes are also denied through `inode_setattr`. File/directory LSM matching is basename-based today; do not describe it as a
recursive path, prefix, glob, or class policy.

## Semantic alerts

The backend currently synthesizes alerts such as:

- `SECRET_ACCESS`
- `SEMANTIC_MISMATCH`
- `UNEXPECTED_NETWORK_EGRESS`
- `UNEXPECTED_CHILD_PROCESS`
- `TOOL_BEHAVIOR_DRIFT`
- `SUSPICIOUS_SHELL_PIPELINE`
- `WORKSPACE_ESCAPE`
- `TOKEN_EXFIL_RISK`
- `RESOURCE_WASTING_LOOP`

These alert classifications are **userspace interpretations** over normalized
events, not direct kernel-enforced decisions by themselves.
They can be used to propose wrapper rules or OS-enforcement map entries, but the
alerts themselves are not in the synchronous cgroup/LSM decision path.

## Important caveat

Do **not** describe the current implementation as a full recursive policy tree or class-based kernel policy engine unless you also change the implementation.

The roadmap still wants richer semantics such as:

- exact / prefix / suffix / class rules
- workspace / secret / system / temp classes
- CIDR / domain network policy
- cgroup attribution
- richer kernel policy classes on top of the existing cgroup hooks and BPF LSM
  exact-map decisions

Those richer semantics are directionally planned, but only the exact
cgroup/IP/port and LSM path/name map decisions described above are implemented
today.
