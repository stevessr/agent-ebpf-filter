# Devcontainer

This Debian-based container is intended for local development of the backend eBPF runtime,
Vue frontend, wrapper, and language adapters.

## Requirements

- Linux Docker host with eBPF-capable kernel and BTF.
- Docker/Podman must allow privileged containers.
- `/sys/fs/bpf`, `/sys/kernel/debug`, and `/lib/modules` are bind-mounted from the host.

The container runs privileged with BPF/PERFMON/SYS_ADMIN capabilities because
`make dev` builds and launches the eBPF backend.

The host user's Git config is passed through read-only:

- `~/.gitconfig` → `/home/vscode/.gitconfig`
- `~/.config/git` → `/home/vscode/.config/git`

Credentials, SSH keys, and Git credential stores are not mounted by this
devcontainer setup.

## Startup

Open the folder in VS Code Dev Containers or compatible tooling. The checked-in
`devcontainer.json` uses the GitHub-built GHCR image directly, not a local
`build` block. If VS Code logs show it inspecting `debian:trixie`, the editor is
using an older config or stale cache.
The container run args include `--pull=always` so recreating the Dev Container
fetches the newest workflow-published image for the current tag.

The config also disables VS Code's remote-user UID rewrite. Otherwise Dev
Containers creates a temporary `updateUID.Dockerfile` and performs a local build
on top of the pulled image. Do not re-enable `updateRemoteUserUID` for this
pull-only workflow. The published image uses `vscode` UID/GID `1001:1001`; the
Podman-backed `docker` CLI maps the host user to that container UID/GID with
`--userns=keep-id:uid=1001,gid=1001` so bind-mounted workspace files remain
writable without creating a derived local image.

`--init` is intentionally omitted because Podman rejects `--init` together with
`--pid=host` (`cannot add init binary as PID 1`). The eBPF workflow keeps
`--pid=host` and relies on the container command itself instead.

By default VS Code pulls:

```bash
ghcr.io/stevessr/agent-ebpf-filter/devcontainer:master-fc613b4dfd67
```

For a branch/fork image, start VS Code with the repository/tag variables set:

```bash
DEV_IMAGE_REPOSITORY="$(make --no-print-directory dev-image-repository)" \
DEV_IMAGE_TAG="$(make --no-print-directory dev-image-tag)" \
code .
```

The GitHub Actions image build runs `make predev` before publishing the GHCR
image, so the published image already contains `protoc-gen-go`, the Python
virtualenv, Node.js for `protobufjs-cli`, and frontend `node_modules`. Because the live workspace is
bind-mounted over `/workspaces/agent-ebpf-filiter`, the image also keeps a copy
of those workspace-local dependencies under `/opt/agent-ebpf-predev`.

The `postCreateCommand` seeds missing or container-unusable workspace-local
dependencies from that image copy and then verifies the setup without installing
anything from the network:

```bash
make predev-check
```

If that check fails, you are probably using a stale GHCR image. Wait for or
rerun the **Devcontainer Image** workflow, pull the updated branch image, and
rebuild/reopen the Dev Container. To opt into an online fallback explicitly,
open the container with `DEVCONTAINER_POSTCREATE_INSTALL=1`.

`make exec` and VS Code Dev Containers mount container-local volumes over
`frontend/node_modules` and `adapters/python/.venv`. That keeps dependency
trees writable inside the container and avoids reusing host-only symlinks from a
host-created Python virtualenv.

CUDA is optional in this image. If the host does not expose `/opt/cuda`, backend
builds compile the CPU-only CUDA stub instead of linking CUDA runtime libraries.

Then start the normal development session:

```bash
make dev
```

`make dev` generates protobuf bindings, builds `agent-wrapper`, and opens the
backend/frontend panes in Zellij. Dev auth is disabled through `DISABLE_AUTH=true`.


## Make targets

Print the branch image that local Make targets use:

```bash
make dev-image
make dev-image-repository
make dev-image-tag
```

`make docker` will pull the GitHub-built branch image from GHCR. The default
image ref is `ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>-<branch-hash>`,
where `branch-slug` is the sanitized branch-name prefix and `branch-hash` is the
first 12 hex characters of the branch name's SHA-256 digest.

Example: `feat/bad` → `ghcr.io/<owner>/<repo>/devcontainer:feat-bad-7dfa0ab55e71`.

```bash
make docker
```

`make exec` creates and starts the privileged container with this repo mounted
at `/workspaces/agent-ebpf-filiter`, passes through the host Git config
read-only, seeds/verifies the prebuilt dependencies, then enters it
automatically with fish. It recreates an older existing devcontainer when needed
to add the read-only Git config mounts. It does not build the image locally. If
the image is missing locally, it pulls it; if the GHCR branch image is missing,
the command fails and tells you to wait for the GitHub Actions devcontainer
image workflow to finish or run that workflow.

```bash
make exec
```

If you need to override the image or container name, keep the same tag format:

```bash
make exec DEV_IMAGE=ghcr.io/<owner>/<repo>/devcontainer:feat-bad-7dfa0ab55e71 DEV_CONTAINER=my-ebpf-dev
```

If you're on a detached HEAD or the branch cannot be inferred, set `DEV_BRANCH=<branch>` or provide a full `DEV_IMAGE=...` before running `make docker` or `make exec`.
