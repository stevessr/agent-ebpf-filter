# Codex Cloud development environment

This is an **unprivileged build/test environment**, separate from `.devcontainer/`, whose existing host mounts and elevated capabilities must not be copied into Codex Cloud. It leaves `.github/workflows/` and deployment unchanged.

## Cloud settings

Repository: `stevessr/agent-ebpf-filter` (default branch `master`). Use Linux x86_64 or arm64, with package-manager network access. Set **Install script** to:

```bash
bash scripts/codex-cloud/install.sh
```

Set **Start skill** to the text in `.agents/skills/codex-cloud-start/SKILL.md`, or use that repository skill where supported. There are no required production secrets or environment variables. Publish the environment only after the install and `bash scripts/codex-cloud/validate.sh full` succeed, and review `reports/codex-cloud/latest.md`.

## Commands

`bash scripts/codex-cloud/detect.sh` inventories tracked manifests and refuses unhandled future Rust/Flutter modules rather than silently skipping them.

```bash
bash scripts/codex-cloud/maintain.sh install   # frozen restore and generation
bash scripts/codex-cloud/maintain.sh doctor    # SDK, native toolchain, lock health
bash scripts/codex-cloud/maintain.sh verify    # lint, vet, tests, builds and report
bash scripts/codex-cloud/maintain.sh start frontend
bash scripts/codex-cloud/maintain.sh start docs
bash scripts/codex-cloud/maintain.sh start status
bash scripts/codex-cloud/maintain.sh stop
bash scripts/codex-cloud/maintain.sh cache
source scripts/codex-cloud/env.sh               # expose pinned tools in a new shell
```

Go version comes from `go.work`, Python version from `adapters/python/.python-version`, Node and Bun from `.devcontainer/Dockerfile`. Go modules are downloaded without editing module files. Bun uses `--frozen-lockfile` in root, frontend and tools/bpf-ts; uv uses `uv.lock` in frozen mode. The protobuf Go generator is explicitly `v1.36.11`, taken from `backend/go.mod`. Rust/Flutter are not installed without a corresponding project manifest; Zellij is optional for Codex Cloud and its Rust build is not needed by this start skill. An existing `uv` is kept; if absent, installer tool pin is `CODEX_UV_VERSION=0.10.9`, overridable without changing Python application dependencies.

Caches under `~/.cache/agent-ebpf-codex` hold Go module/build data, Bun, uv and managed Python. Pinned SDKs are installed under `~/.local/share/agent-ebpf-codex`; unchanged versions are reused. Persistence across separately provisioned cloud snapshots depends on Codex Cloud's actual environment snapshot/cache policy, not a guarantee from the script.

## Prerequisites and limitations

Install needs apt on Debian/Ubuntu with root or passwordless sudo, unless native build tools/headers are already available; external downloads require access to system apt mirrors, `go.dev`, `nodejs.org`, `bun.sh`, `proxy.golang.org`, `sum.golang.org`, npm/Bun registry, PyPI, and GitHub/astral-sh Python release assets used by uv. Optional Git submodules are **not** fetched by default: use `CODEX_FETCH_SUBMODULES=1` for the large documentation references. No real credentials are used or written.

Live kernel verifier/attach, tracepoints, BPF LSM and cgroup smoke tests are explicitly **NOT RUN** in the rootless cloud environment; run those separately on an authorized eBPF/BTF-capable Linux machine. The frontend can boot alone, but live API screens require a separately running backend. Any install, test, or compile failure remains a failure in `reports/codex-cloud/latest.md` and its logs. Reports, caches and generated build outputs must not be committed.

The Cloud environment itself is selected, tested, and published in the Codex Cloud UI; committing these files does not publish a cloud environment automatically.
