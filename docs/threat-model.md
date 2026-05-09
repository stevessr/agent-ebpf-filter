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

## Non-goals

- defending against a root attacker
- defending against a malicious kernel
- full container-escape prevention
- complete prevention of every local persistence or post-exploitation technique

Those require stronger sandboxing, kernel hardening, or external isolation layers beyond the current scope.
