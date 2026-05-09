# Codex-Style Workflows

This repo is being shaped around **agent-runtime evidence** for modern coding-agent workflows.

## Target workflows

- local CLI coding agents
- cloud / delegated tasks
- PR review loops
- parallel tool calls
- mid-turn steering and follow-up tasks
- IDE handoff
- remote devbox / SSH usage
- browser / frontend iteration
- MCP-backed tool execution

## What the runtime plane should capture

For each workflow, the project tries to correlate:

1. declared semantic action (`tool_name`, hook, wrapper request, MCP call)
2. process tree and child inheritance
3. file access and mutations
4. network egress
5. policy decisions and semantic alerts
6. exit / wait lifecycle

## Practical examples

### PR review

Expected:

- mostly read-only file access
- no unrelated secret access
- no unexplained network egress

### Dependency install

Expected:

- package-manager child processes
- registry network access
- workspace / cache writes

### Frontend/browser iteration

Expected:

- local dev server processes
- workspace writes
- local preview/network traffic

Unexpected:

- `ssh`, `nc`, `socat`, raw reverse-shell behavior
- secret reads unrelated to the current task

### MCP tool call

Expected:

- tool identity attached to the run context
- child processes and network egress attributable back to that tool call

Unexpected:

- download-and-exec pivots
- hidden external egress unrelated to the declared tool behavior

## Current repo support

- inherited `agent_run_id` / `task_id` / `tool_call_id` / `trace_id`
- execution graph UI
- semantic mismatch alerts
- envelope streaming
- Prometheus + OTLP export
- offline replay benchmarks for benign / malicious / agentic scenarios

The remaining roadmap still includes stronger cgroup / LSM enforcement and a real privileged-vs-unprivileged split.
