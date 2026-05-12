# Devcontainer

This Debian-based container is intended for local development of the backend eBPF runtime,
Vue frontend, wrapper, and language adapters.

## Requirements

- Linux Docker host with eBPF-capable kernel and BTF.
- Docker/Podman must allow privileged containers.
- `/sys/fs/bpf`, `/sys/kernel/debug`, and `/lib/modules` are bind-mounted from the host.

The container runs privileged with BPF/PERFMON/SYS_ADMIN capabilities because
`make dev` builds and launches the eBPF backend.

## Startup

Open the folder in VS Code Dev Containers or compatible tooling. The
`postCreateCommand` runs:

```bash
make predev
```

Then start the normal development session:

```bash
make dev
```

`make dev` generates protobuf bindings, builds `agent-wrapper`, and opens the
backend/frontend panes in Zellij. Dev auth is disabled through `DISABLE_AUTH=true`.


## Make targets

`make docker` will pull the GitHub-built branch image from GHCR. The default
image ref is `ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>-<branch-hash>`,
where `branch-slug` is the sanitized branch-name prefix and `branch-hash` is the
first 12 hex characters of the branch name's SHA-256 digest.

Example: `feat/bad` → `ghcr.io/<owner>/<repo>/devcontainer:feat-bad-7dfa0ab55e71`.

```bash
make docker
```

`make exec` creates and starts the privileged container with this repo mounted
at `/workspaces/agent-ebpf-filiter`, then enters it automatically with fish. It
does not build the image locally. If the image is missing locally, it pulls it;
if the GHCR branch image is missing, the command fails and tells you to wait for
the GitHub Actions devcontainer image workflow to finish or run that workflow.

```bash
make exec
```

If you need to override the image or container name, keep the same tag format:

```bash
make exec DEV_IMAGE=ghcr.io/<owner>/<repo>/devcontainer:feat-bad-7dfa0ab55e71 DEV_CONTAINER=my-ebpf-dev
```

If you're on a detached HEAD or the branch cannot be inferred, set `DEV_BRANCH=<branch>` or provide a full `DEV_IMAGE=...` before running `make docker` or `make exec`.
