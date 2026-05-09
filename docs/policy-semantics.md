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

These alerts are **userspace interpretations** over normalized events. They are not kernel-enforced policy decisions yet.

## Important caveat

Do **not** describe the current implementation as a full recursive policy tree or class-based kernel policy engine unless you also change the implementation.

The roadmap still wants richer semantics such as:

- exact / prefix / suffix / class rules
- workspace / secret / system / temp classes
- CIDR / domain network policy
- cgroup attribution
- optional kernel blocking via cgroup hooks or BPF LSM

Those are directionally planned, but only partially implemented today.
