# GHCR Devcontainer Build Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move devcontainer image building to GitHub Actions and make local development pull the branch-tagged GHCR image instead of building it locally.

**Architecture:** GitHub Actions builds `.devcontainer/Dockerfile` and pushes `ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>`. The `Makefile` derives the same branch slug locally, makes `make docker` pull-only, and keeps `make exec` responsible for starting or attaching to the privileged container. Documentation explains the new GHCR workflow and the pull-failure behavior.

**Tech Stack:** GitHub Actions, GHCR, Docker Buildx, GNU Make, Docker/Podman, Debian devcontainer.

---

## File structure

- Create: `.github/workflows/devcontainer-image.yml`
  - Builds `.devcontainer/Dockerfile` on branch pushes and manual dispatch.
  - Logs in to GHCR with `GITHUB_TOKEN`.
  - Publishes a branch-slug tag matching the local Makefile slug rule.
- Modify: `Makefile:5-34`
  - Add local branch/repository/image reference variables.
  - Change `make docker` from local build to GHCR pull.
  - Keep `make exec` startup flow, but ensure missing images trigger pull-only behavior.
- Modify: `Makefile:234-253`
  - Remove the legacy `make docker clean` special case that described Docker build-cache cleanup.
- Modify: `.devcontainer/README.md:34-59`
  - Replace local-build instructions with GHCR pull-only instructions.
  - Document branch tags, override variables, and pull-failure behavior.
- Modify: `AGENTS.md:24-43`
  - Update build workflow notes so agents know `make docker` pulls GHCR images and does not build locally.
- Modify: `README.md:242-254`
  - Add `make docker` and `make exec` to useful targets with pull-only wording.

Implementation note: Do not create git commits during execution unless the user explicitly asks for commits. The plan has verification checkpoints instead of mandatory commit steps to respect the repository operating rule.

---

### Task 1: Add the GHCR image workflow

**Files:**
- Create: `.github/workflows/devcontainer-image.yml`

- [ ] **Step 1: Create the workflow directory if it does not exist**

Run:

```bash
ls .github
```

Expected if `.github` is missing: `ls` reports `No such file or directory`.

If it is missing, create only the workflow directory:

```bash
mkdir -p .github/workflows
```

Expected: command exits with status 0.

- [ ] **Step 2: Write the workflow file**

Create `.github/workflows/devcontainer-image.yml` with exactly this content:

```yaml
name: Devcontainer Image

on:
  push:
    branches:
      - "**"
    paths:
      - ".devcontainer/**"
      - ".github/workflows/devcontainer-image.yml"
  workflow_dispatch:

permissions:
  contents: read
  packages: write

concurrency:
  group: devcontainer-image-${{ github.ref }}
  cancel-in-progress: true

jobs:
  build:
    name: Build and publish devcontainer
    runs-on: ubuntu-latest

    steps:
      - name: Check out repository
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Compute image metadata
        id: meta
        shell: bash
        run: |
          slug="$(printf '%s' "${GITHUB_REF_NAME}" | sed -E 's#[^A-Za-z0-9_.-]+#-#g; s#^-+##; s#-+$##')"
          if [ -z "$slug" ]; then
            slug="${GITHUB_SHA::12}"
          fi
          repo="$(printf '%s' "${GITHUB_REPOSITORY}" | tr '[:upper:]' '[:lower:]')"
          echo "image=ghcr.io/${repo}/devcontainer" >> "$GITHUB_OUTPUT"
          echo "tag=${slug}" >> "$GITHUB_OUTPUT"

      - name: Build and push devcontainer image
        uses: docker/build-push-action@v6
        with:
          context: .devcontainer
          file: .devcontainer/Dockerfile
          push: true
          tags: ${{ steps.meta.outputs.image }}:${{ steps.meta.outputs.tag }}
          build-args: |
            GO_VERSION=1.26.2
            USER_UID=1001
            USER_GID=1001
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

- [ ] **Step 3: Verify branch slug logic locally**

Run:

```bash
branch='feat/bad'; slug="$(printf '%s' "$branch" | sed -E 's#[^A-Za-z0-9_.-]+#-#g; s#^-+##; s#-+$##')"; printf '%s\n' "$slug"
```

Expected output:

```text
feat-bad
```

- [ ] **Step 4: Verify workflow syntax is readable YAML text**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
path = Path('.github/workflows/devcontainer-image.yml')
text = path.read_text()
required = [
    'name: Devcontainer Image',
    'registry: ghcr.io',
    'docker/build-push-action@v6',
    'tags: ${{ steps.meta.outputs.image }}:${{ steps.meta.outputs.tag }}',
]
missing = [item for item in required if item not in text]
if missing:
    raise SystemExit(f'missing required workflow text: {missing}')
print('workflow text checks passed')
PY
```

Expected output:

```text
workflow text checks passed
```

---

### Task 2: Make local devcontainer image handling pull-only

**Files:**
- Modify: `Makefile:5-34`
- Modify: `Makefile:234-253`

- [ ] **Step 1: Replace the devcontainer variables at the top of `Makefile`**

Replace lines 5-11 with this block:

```make
CONTAINER_CLI ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
DEV_BRANCH ?= $(shell git branch --show-current 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || printf local)
DEV_IMAGE_TAG ?= $(shell branch='$(DEV_BRANCH)'; slug=$$(printf '%s' "$$branch" | sed -E 's#[^A-Za-z0-9_.-]+#-#g; s#^-+##; s#-+$$##'); printf '%s' "$${slug:-local}")
DEV_IMAGE_OWNER_REPO ?= $(shell git config --get remote.origin.url 2>/dev/null | sed -E 's#^git@github.com:#https://github.com/#; s#^ssh://git@github.com/##; s#^https://github.com/##; s#\.git$$##' | tr '[:upper:]' '[:lower:]')
DEV_IMAGE_REPOSITORY ?= ghcr.io/$(DEV_IMAGE_OWNER_REPO)/devcontainer
DEV_IMAGE ?= $(DEV_IMAGE_REPOSITORY):$(DEV_IMAGE_TAG)
DEV_CONTAINER ?= agent-ebpf-filiter-dev
DEV_WORKSPACE ?= /workspaces/agent-ebpf-filiter
DEVCONTAINER_GO_VERSION ?= 1.26.2
DEVCONTAINER_USER_UID ?= 1001
DEVCONTAINER_USER_GID ?= 1001
```

- [ ] **Step 2: Replace the `docker` target**

Replace the existing target that starts with `docker: ## Build the privileged devcontainer image` and ends before `exec:` with this block:

```make
docker: ## Pull the privileged devcontainer image from GHCR
	@test -n "$(CONTAINER_CLI)" || (echo "Missing docker or podman CLI." && exit 1)
	@if [ "$(DEV_IMAGE)" = "ghcr.io//devcontainer:$(DEV_IMAGE_TAG)" ]; then \
		echo "Cannot infer GitHub owner/repo from origin remote."; \
		echo "Set DEV_IMAGE=ghcr.io/<owner>/<repo>/devcontainer:$(DEV_IMAGE_TAG) and retry."; \
		exit 1; \
	fi
	@echo "Pulling $(DEV_IMAGE)..."
	@if ! $(CONTAINER_CLI) pull $(DEV_IMAGE); then \
		echo "Failed to pull $(DEV_IMAGE)."; \
		echo "Wait for or run the GitHub Actions devcontainer image workflow for branch '$(DEV_BRANCH)' and retry."; \
		echo "Local devcontainer builds are intentionally disabled."; \
		exit 1; \
	fi
```

- [ ] **Step 3: Keep `exec` pull-only through the existing missing-image path**

Confirm the `exec` target still contains this line exactly:

```make
	@$(CONTAINER_CLI) image inspect $(DEV_IMAGE) >/dev/null 2>&1 || $(MAKE) --no-print-directory docker
```

Do not add any `docker build`, `podman build`, `$(CONTAINER_CLI) build`, or `.devcontainer/Dockerfile` command to `exec`.

- [ ] **Step 4: Remove the legacy docker-clean special case**

Replace the `clean` target body at `Makefile:234-253` with this block:

```make
clean: ## Clean build artifacts
	rm -f backend/agent-ebpf-filter
	rm -f agent-wrapper
	rm -f backend/.port
	rm -rf frontend/dist
	rm -rf adapters/python/.venv
	rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go
	rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o
	rm -f backend/ebpf/agenttlscapture_x86_bpfel.go backend/ebpf/agenttlscapture_x86_bpfeb.go
	rm -f backend/ebpf/agenttlscapture_x86_bpfel.o backend/ebpf/agenttlscapture_x86_bpfeb.o
	rm -f backend/ebpf/agentcgroupsandbox_bpfel.go backend/ebpf/agentcgroupsandbox_bpfeb.go
	rm -f backend/ebpf/agentcgroupsandbox_bpfel.o backend/ebpf/agentcgroupsandbox_bpfeb.o
	rm -f backend/ebpf/agentlsmenforcer_bpfel.go backend/ebpf/agentlsmenforcer_bpfeb.go
	rm -f backend/ebpf/agentlsmenforcer_bpfel.o backend/ebpf/agentlsmenforcer_bpfeb.o
	rm -rf backend/pb
	rm -rf frontend/src/pb
```

- [ ] **Step 5: Verify the Makefile dry-run pulls the expected branch image**

Run:

```bash
make -n docker CONTAINER_CLI=true DEV_IMAGE_OWNER_REPO=stevessr/agent-ebpf-filiter DEV_BRANCH=feat/bad
```

Expected output contains these lines or their shell-expanded equivalents:

```text
Pulling ghcr.io/stevessr/agent-ebpf-filiter/devcontainer:feat-bad...
true pull ghcr.io/stevessr/agent-ebpf-filiter/devcontainer:feat-bad
```

- [ ] **Step 6: Verify there is no local build path in the devcontainer targets**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
text = Path('Makefile').read_text()
start = text.index('docker: ## Pull the privileged devcontainer image from GHCR')
end = text.index('\nall:', start)
block = text[start:end]
for forbidden in ['$(CONTAINER_CLI) build', 'docker build', 'podman build', '.devcontainer/Dockerfile']:
    if forbidden in block:
        raise SystemExit(f'forbidden local build reference remains: {forbidden}')
print('pull-only Makefile checks passed')
PY
```

Expected output:

```text
pull-only Makefile checks passed
```

---

### Task 3: Update devcontainer documentation

**Files:**
- Modify: `.devcontainer/README.md:34-59`

- [ ] **Step 1: Replace the Make targets section**

Replace `.devcontainer/README.md` from `## Make targets` through the end of the file with this content:

```markdown
## Make targets

From the host, pull the GitHub-built branch image from GHCR:

```bash
make docker
```

The default image reference is derived from the GitHub origin remote and current branch:

```text
ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>
```

For example, branch `feat/bad` pulls:

```text
ghcr.io/<owner>/<repo>/devcontainer:feat-bad
```

Create/start the privileged container with this repo mounted at
`/workspaces/agent-ebpf-filiter` and enter it automatically with fish:

```bash
make exec
```

`make exec` pulls the branch image when it is missing locally. If the image does
not exist in GHCR yet, the command fails and asks you to wait for or run the
GitHub Actions devcontainer image workflow. It does not build the image locally.

Override names when needed:

```bash
make exec DEV_IMAGE=ghcr.io/example/agent-ebpf-filiter/devcontainer:feat-bad DEV_CONTAINER=my-ebpf-dev
```
```

- [ ] **Step 2: Verify the documentation no longer advertises local image builds**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
text = Path('.devcontainer/README.md').read_text()
for forbidden in ['build the same image', 'build cache', 'make docker clean']:
    if forbidden in text:
        raise SystemExit(f'legacy devcontainer wording remains: {forbidden}')
for required in ['pull the GitHub-built branch image from GHCR', 'It does not build the image locally']:
    if required not in text:
        raise SystemExit(f'missing required wording: {required}')
print('devcontainer docs checks passed')
PY
```

Expected output:

```text
devcontainer docs checks passed
```

---

### Task 4: Update contributor and README workflow notes

**Files:**
- Modify: `AGENTS.md:24-45`
- Modify: `README.md:242-254`

- [ ] **Step 1: Update `AGENTS.md` build workflow list**

In `AGENTS.md`, replace the command block under `## 3. Build and regeneration workflow` with this block:

```markdown
```bash
rtk make help
rtk make docker       # Pull GHCR devcontainer image for the current branch; no local image build
rtk make exec         # Start/attach to the privileged devcontainer shell
rtk make predev
rtk make proto
rtk make backend
rtk make wrapper
rtk make frontend
rtk make runtime-benchmark
rtk make ebpf-cgroup
rtk make ebpf-lsm
rtk make os-enforcement-preflight
rtk make os-enforcement-check
rtk make os-enforcement-smoke
rtk env OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start
rtk make dev
```
```

After the paragraph that starts with `` `make predev` installs``, add this paragraph:

```markdown
`make docker` is pull-only: it derives `ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>` from the GitHub origin remote and current branch, then pulls that GHCR image. If the image is unavailable, wait for or run the GitHub Actions devcontainer image workflow; do not add a local build fallback.
```

- [ ] **Step 2: Update README useful targets**

In `README.md`, replace the useful targets block at lines 244-254 with this block:

```markdown
```bash
make help
make docker      # Pull the GitHub-built devcontainer image for this branch
make exec        # Start or attach to the privileged devcontainer shell
make proto
make backend
make wrapper
make frontend
make runtime-benchmark
make run-backend
make run-frontend
make clean
```
```

- [ ] **Step 3: Verify documentation contains the GHCR workflow guidance**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
checks = {
    'AGENTS.md': [
        'rtk make docker       # Pull GHCR devcontainer image for the current branch; no local image build',
        'do not add a local build fallback',
    ],
    'README.md': [
        'make docker      # Pull the GitHub-built devcontainer image for this branch',
        'make exec        # Start or attach to the privileged devcontainer shell',
    ],
}
for path, needles in checks.items():
    text = Path(path).read_text()
    for needle in needles:
        if needle not in text:
            raise SystemExit(f'{path} missing: {needle}')
print('repository docs checks passed')
PY
```

Expected output:

```text
repository docs checks passed
```

---

### Task 5: Final verification

**Files:**
- Verify: `.github/workflows/devcontainer-image.yml`
- Verify: `Makefile`
- Verify: `.devcontainer/README.md`
- Verify: `AGENTS.md`
- Verify: `README.md`

- [ ] **Step 1: Check changed files**

Run:

```bash
git status --short
```

Expected output includes these paths:

```text
?? .github/workflows/devcontainer-image.yml
 M Makefile
 M README.md
 M AGENTS.md
 M .devcontainer/README.md
```

The repository already has unrelated modified and untracked files; do not stage or delete them.

- [ ] **Step 2: Verify branch image computation**

Run:

```bash
make -n docker CONTAINER_CLI=true DEV_IMAGE_OWNER_REPO=stevessr/agent-ebpf-filiter DEV_BRANCH=feat/bad
```

Expected output includes:

```text
ghcr.io/stevessr/agent-ebpf-filiter/devcontainer:feat-bad
```

- [ ] **Step 3: Verify `make exec` still delegates missing images to `make docker`**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
text = Path('Makefile').read_text()
required = '@$(CONTAINER_CLI) image inspect $(DEV_IMAGE) >/dev/null 2>&1 || $(MAKE) --no-print-directory docker'
if required not in text:
    raise SystemExit('make exec no longer delegates missing image pulls to make docker')
print('make exec delegation check passed')
PY
```

Expected output:

```text
make exec delegation check passed
```

- [ ] **Step 4: Verify no local devcontainer build commands remain in local Makefile image flow**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
text = Path('Makefile').read_text()
start = text.index('docker: ## Pull the privileged devcontainer image from GHCR')
end = text.index('\nall:', start)
block = text[start:end]
for forbidden in ['$(CONTAINER_CLI) build', 'docker build', 'podman build']:
    if forbidden in block:
        raise SystemExit(f'forbidden local build command remains: {forbidden}')
print('no local build commands remain in docker/exec flow')
PY
```

Expected output:

```text
no local build commands remain in docker/exec flow
```

- [ ] **Step 5: Check whitespace in changed files**

Run:

```bash
git diff --check -- Makefile README.md AGENTS.md .devcontainer/README.md .github/workflows/devcontainer-image.yml
```

Expected output: no output and exit status 0.

- [ ] **Step 6: Report the remaining remote verification**

Tell the user:

```text
Local Makefile and documentation checks passed. Full GHCR publishing still needs a GitHub Actions run after pushing the branch, because local verification cannot prove remote package permissions or GHCR push behavior.
```
