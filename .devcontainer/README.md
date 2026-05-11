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

From the host, build the same image without VS Code:

```bash
make docker
```

Remove the Docker/Podman build cache used by the dev image:

```bash
make docker clean
```

Create/start the privileged container with this repo mounted at
`/workspaces/agent-ebpf-filiter` and enter it automatically with fish:

```bash
make exec
```

Override names when needed:

```bash
make exec DEV_IMAGE=my-ebpf-dev DEV_CONTAINER=my-ebpf-dev
```
