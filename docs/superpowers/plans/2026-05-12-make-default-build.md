# Make Default Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make bare `make` run the local build output path instead of building the devcontainer image.

**Architecture:** Use GNU Make's explicit `.DEFAULT_GOAL` setting to decouple default target selection from the first target in the file. Keep the existing `docker` and `exec` targets available for explicit container workflows.

**Tech Stack:** GNU Make, existing project Makefile targets.

---

### Task 1: Set default Make target to local build

**Files:**
- Modify: `Makefile:1-6`

- [ ] **Step 1: Add explicit default goal**

Insert `.DEFAULT_GOAL := build` near the top of `Makefile`, before the first real target:

```make
.DEFAULT_GOAL := build

# Get Go binaries path
GOPATH ?= $(shell go env GOPATH)
export PATH := $(PATH):$(GOPATH)/bin
```

- [ ] **Step 2: Verify dry-run default target**

Run:

```bash
make -n
```

Expected: output starts with `make --no-print-directory -j3...` or the commands from the `build` dependency chain, and does not execute the `docker` target's `$(CONTAINER_CLI) build` command as the default goal.

- [ ] **Step 3: Confirm explicit Docker target remains available**

Run:

```bash
make -n docker
```

Expected: output still shows the Docker/Podman image build commands for `.devcontainer/Dockerfile`.
