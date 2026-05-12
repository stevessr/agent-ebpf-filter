# Threat Model

This project aims to protect the **agent execution chain**, not just individual commands.

## Protected assets

- workspace files and generated artifacts
- secrets and credential material (`~/.ssh`, `.env`, cloud credentials, kube configs, tokens)
- host filesystem boundaries outside the intended workspace
- network egress from agent-controlled processes
- the user shell / wrapper control path
- PR review and delegated-task execution traces

## Threat sources

- prompt injection in local files, web pages, issues, PRs, docs, or copied snippets
- malicious dependencies or install scripts
- malicious MCP tools / tool servers
- compromised or over-permissioned coding agents
- rogue local helper processes spawned by the agent
- malicious filenames, branch names, shell escaping, or Unicode tricks
- remote-devbox / SSH misuse
- browser / plugin / frontend iteration abuse

## What the system tries to prove

The security plane should answer four questions for each run:

1. **What did the agent say it was doing?**
2. **What processes actually ran?**
3. **What files / network endpoints were touched?**
4. **What policy or semantic alerts fired, and why?**

## Current detection focus

- child-process inheritance from a registered parent
- file access and secret-path hints
- network egress visibility
- semantic mismatch between declared tool intent and observed OS behavior
- wrapper / hook / eBPF fact correlation

## Current OS-level enforcement focus

The repo now has two deterministic kernel-enforced deny paths in addition to
wrapper and hook decisions:

- cgroup/connect and cgroup/sendmsg programs can reject matching outbound TCP
  connects, UDP connected-socket connects, existing connected UDP sends, and
  unconnected UDP sendto/sendmsg for explicit cgroup ids, IPv4/IPv6
  destinations, IPv4-mapped IPv6 destinations, or destination ports before the
  kernel operation completes.
- BPF LSM programs can reject explicit executable paths / executable basenames
  and file or directory basenames at `exec`, `open`, existing-fd read/write,
  `mmap`, `mprotect`, existing-fd `ftruncate` / `fchmod` via `setattr`,
  `create`, `link`, `symlink`, `unlink`, `mkdir`, `rmdir`, `mknod`, and
  `rename` decision points.

These maps start empty and are mutated through authenticated policy APIs. They
are not recursive workspace sandboxes, CIDR/domain policy engines, or container
escape defenses.

## Non-goals

- defending against a root attacker
- defending against a malicious kernel
- full container-escape prevention
- complete prevention of every local persistence or post-exploitation technique
- broad filesystem/network sandbox policy beyond the explicit cgroup/LSM map
  entries described above

Those require broader sandboxing, kernel hardening, or external isolation layers
beyond the current explicit OS-enforcement maps.
