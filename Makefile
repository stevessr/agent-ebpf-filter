# Get a writable Go workspace for helper binaries and the module cache.
GO ?= $(shell command -v go 2>/dev/null || command -v /usr/local/go/bin/go 2>/dev/null || printf go)
GO_PATH_DIR := $(if $(filter /%,$(GO)),$(patsubst %/,%,$(dir $(GO))),)
GO_SAFE_GOPATH := $(shell \
	p="$$($(GO) env GOPATH 2>/dev/null || true)"; \
	[ -n "$$p" ] || p="$$HOME/go"; \
	if mkdir -p "$$p/bin" "$$p/pkg/mod" 2>/dev/null \
		&& [ -w "$$p/bin" ] \
		&& [ -w "$$p/pkg/mod" ]; then \
		printf '%s' "$$p"; \
	else \
		printf '%s' "$$HOME/go"; \
	fi)
ifneq ($(origin GOPATH), command line)
GOPATH := $(GO_SAFE_GOPATH)
endif
export GOPATH
export PATH := $(PATH)$(if $(GO_PATH_DIR),:$(GO_PATH_DIR)):$(GOPATH)/bin

CONTAINER_CLI ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
DEV_BRANCH ?= $(strip $(shell git branch --show-current 2>/dev/null))
DEV_IMAGE_TAG ?= $(shell branch="$(DEV_BRANCH)"; base=$$(printf '%s' "$$branch" | sed -E 's#[^A-Za-z0-9_.-]+#-#g; s#^-+##; s#-+$$##'); [ -n "$$base" ] || base=local; suffix=$$(printf '%s' "$$branch" | sha256sum | cut -c1-12); prefix=$$(printf '%s' "$$base" | cut -c1-115 | sed -E 's#-+$$##'); [ -n "$$prefix" ] || prefix=local; printf '%s-%s' "$$prefix" "$$suffix")
DEV_IMAGE_OWNER_REPO ?= $(shell git remote get-url origin 2>/dev/null | sed -E 's#^(git@github.com:|https://github.com/|ssh://git@github.com/)##; s#\.git$$##' | tr '[:upper:]' '[:lower:]')
DEV_IMAGE_REPOSITORY ?= ghcr.io/$(DEV_IMAGE_OWNER_REPO)/devcontainer
DEV_IMAGE ?= $(DEV_IMAGE_REPOSITORY):$(DEV_IMAGE_TAG)
DEV_CONTAINER ?= agent-ebpf-filiter-dev
DEV_WORKSPACE ?= /workspaces/agent-ebpf-filiter
DEVCONTAINER_GO_VERSION ?= 1.26.2
DEVCONTAINER_USER_UID ?= 1001
DEVCONTAINER_USER_GID ?= 1001
DEV_CONTAINER_USERNS ?= $(shell $(CONTAINER_CLI) --version 2>/dev/null | grep -qi podman && printf 'keep-id:uid=$(DEVCONTAINER_USER_UID),gid=$(DEVCONTAINER_USER_GID)')
DEV_CONTAINER_USERNS_ARG = $(if $(DEV_CONTAINER_USERNS),--userns=$(DEV_CONTAINER_USERNS),)
DEV_FRONTEND_NODE_MODULES_VOLUME ?= $(DEV_CONTAINER)-frontend-node-modules
DEV_PYTHON_VENV_VOLUME ?= $(DEV_CONTAINER)-python-venv
CUDA_GO_TAGS ?= $(shell [ -x /opt/cuda/bin/nvcc ] && [ -r /opt/cuda/lib64/libcudart.so ] && printf cuda)
GO_BUILD_TAGS_ARG = $(if $(strip $(CUDA_GO_TAGS)),-tags "$(CUDA_GO_TAGS)",)
INSTALL_PREFIX ?= /opt/agent-ebpf-filter
INSTALL_BINDIR ?= /usr/local/bin
INSTALL_SYSCONFDIR ?= /etc/agent-ebpf-filter
INSTALL_SERVICE_NAME ?= agent-ebpf-filter
INSTALL_METHOD ?= auto
INSTALL_ENABLE ?= 1
INSTALL_START ?= 1

.DEFAULT_GOAL := all

.PHONY: all backend frontend wrapper clean proto proto-check help predev predev-check predev-go predev-python predev-frontend dev run deps ebpf-bootstrap ebpf-tls ebpf-cgroup ebpf-lsm os-enforcement-preflight os-enforcement-check os-enforcement-smoke os-enforcement-smoke-start cuda ml-sweep ml-presentation runtime-benchmark test build install uninstall docker dev-image dev-image-repository dev-image-tag exec


docker: ## Pull the privileged devcontainer image from GHCR
	@test -n "$(CONTAINER_CLI)" || (echo "Missing docker or podman CLI." && exit 1)
	@if [ -z "$(DEV_BRANCH)" ] && [ "$(origin DEV_IMAGE)" = "file" ]; then \
		echo "Cannot infer current git branch."; \
		echo "Set DEV_BRANCH=<branch> or DEV_IMAGE=ghcr.io/<owner>/<repo>/devcontainer:<tag> and retry."; \
		exit 1; \
	fi
	@if [ "$(DEV_IMAGE)" = "ghcr.io//devcontainer:$(DEV_IMAGE_TAG)" ]; then \
		echo "Cannot derive owner/repo from origin remote."; \
		echo "Set DEV_IMAGE=ghcr.io/<owner>/<repo>/devcontainer:$(DEV_IMAGE_TAG)"; \
		exit 1; \
	fi
	@echo "Pulling $(DEV_IMAGE)..."
	@$(CONTAINER_CLI) pull $(DEV_IMAGE) || { \
		echo "Failed to pull $(DEV_IMAGE)."; \
		if [ "$(origin DEV_IMAGE)" = "file" ]; then \
			echo "Wait for or run the GitHub Actions devcontainer image workflow for branch '$(DEV_BRANCH)' and retry."; \
		else \
			echo "Verify that the supplied DEV_IMAGE exists and that your container CLI can access it."; \
		fi; \
		echo "Local builds are disabled."; \
		exit 1; \
	}

dev-image: ## Print the GHCR devcontainer image for the current branch
	@echo "$(DEV_IMAGE)"

dev-image-repository: ## Print the GHCR devcontainer image repository
	@echo "$(DEV_IMAGE_REPOSITORY)"

dev-image-tag: ## Print the GHCR devcontainer image tag for the current branch
	@echo "$(DEV_IMAGE_TAG)"

exec: ## Start or attach to the mounted devcontainer shell
	@test -n "$(CONTAINER_CLI)" || (echo "Missing docker or podman CLI." && exit 1)
	@$(CONTAINER_CLI) image inspect $(DEV_IMAGE) >/dev/null 2>&1 || $(MAKE) --no-print-directory docker
	@recreate=false; \
	recreate_reason=""; \
	if $(CONTAINER_CLI) container inspect $(DEV_CONTAINER) >/dev/null 2>&1; then \
		if [ -n "$(DEV_CONTAINER_USERNS)" ] && ! $(CONTAINER_CLI) inspect -f '{{json .HostConfig.IDMappings.UIDMap}}' $(DEV_CONTAINER) | grep -F '"$(DEVCONTAINER_USER_UID):' >/dev/null; then \
			recreate=true; \
			recreate_reason="$$recreate_reason user namespace mapping for uid $(DEVCONTAINER_USER_UID)"; \
		fi; \
		if [ -f "$$HOME/.gitconfig" ] && ! $(CONTAINER_CLI) inspect -f '{{range .Mounts}}{{if eq .Destination "/home/vscode/.gitconfig"}}{{if not .RW}}readonly{{end}}{{end}}{{end}}' $(DEV_CONTAINER) | grep -qx readonly; then \
			recreate=true; \
			recreate_reason="$$recreate_reason read-only ~/.gitconfig"; \
		fi; \
		if [ -d "$$HOME/.config/git" ] && ! $(CONTAINER_CLI) inspect -f '{{range .Mounts}}{{if eq .Destination "/home/vscode/.config/git"}}{{if not .RW}}readonly{{end}}{{end}}{{end}}' $(DEV_CONTAINER) | grep -qx readonly; then \
			recreate=true; \
			recreate_reason="$$recreate_reason read-only ~/.config/git"; \
		fi; \
		if ! $(CONTAINER_CLI) inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' $(DEV_CONTAINER) | grep -Fx "$(DEV_WORKSPACE)/frontend/node_modules" >/dev/null; then \
			recreate=true; \
			recreate_reason="$$recreate_reason frontend node_modules volume"; \
		fi; \
		if ! $(CONTAINER_CLI) inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' $(DEV_CONTAINER) | grep -Fx "$(DEV_WORKSPACE)/adapters/python/.venv" >/dev/null; then \
			recreate=true; \
			recreate_reason="$$recreate_reason Python virtualenv volume"; \
		fi; \
		if [ "$$recreate" = "true" ]; then \
			echo "Recreating existing container $(DEV_CONTAINER) to update mounts:$$recreate_reason"; \
			$(CONTAINER_CLI) rm -f $(DEV_CONTAINER) >/dev/null; \
		fi; \
	fi; \
	if $(CONTAINER_CLI) container inspect $(DEV_CONTAINER) >/dev/null 2>&1; then \
		if [ "$$($(CONTAINER_CLI) inspect -f '{{.State.Running}}' $(DEV_CONTAINER))" != "true" ]; then \
			echo "Starting existing container $(DEV_CONTAINER)..."; \
			$(CONTAINER_CLI) start $(DEV_CONTAINER) >/dev/null; \
		fi; \
	else \
		git_mount_args=""; \
		if [ -f "$$HOME/.gitconfig" ]; then \
			git_mount_args="$$git_mount_args --mount type=bind,source=$$HOME/.gitconfig,target=/home/vscode/.gitconfig,readonly"; \
		else \
			echo "Host $$HOME/.gitconfig not found; skipping Git config file mount."; \
		fi; \
		if [ -d "$$HOME/.config/git" ]; then \
			git_mount_args="$$git_mount_args --mount type=bind,source=$$HOME/.config/git,target=/home/vscode/.config/git,readonly"; \
		fi; \
		echo "Creating container $(DEV_CONTAINER) from $(DEV_IMAGE)..."; \
		$(CONTAINER_CLI) run -dit \
			--name $(DEV_CONTAINER) \
			$(DEV_CONTAINER_USERNS_ARG) \
			--privileged \
			--cap-add SYS_ADMIN \
			--cap-add SYS_RESOURCE \
			--cap-add SYS_PTRACE \
			--cap-add NET_ADMIN \
			--cap-add BPF \
			--cap-add PERFMON \
			--security-opt apparmor=unconfined \
			--security-opt seccomp=unconfined \
			--pid=host \
			--network=host \
			-e GIN_MODE=debug \
			-e DISABLE_AUTH=true \
			-e BUN_INSTALL=/usr/local/bun \
			-v "$(CURDIR):$(DEV_WORKSPACE)" \
			--mount type=volume,source=$(DEV_FRONTEND_NODE_MODULES_VOLUME),target=$(DEV_WORKSPACE)/frontend/node_modules \
			--mount type=volume,source=$(DEV_PYTHON_VENV_VOLUME),target=$(DEV_WORKSPACE)/adapters/python/.venv \
			-v /sys/kernel/debug:/sys/kernel/debug \
			-v /sys/fs/bpf:/sys/fs/bpf \
			-v /lib/modules:/lib/modules:ro \
			$$git_mount_args \
			-w $(DEV_WORKSPACE) \
			$(DEV_IMAGE) fish >/dev/null; \
	fi
	@$(CONTAINER_CLI) exec -w $(DEV_WORKSPACE) $(DEV_CONTAINER) bash .devcontainer/post-create.sh
	$(CONTAINER_CLI) exec -it -w $(DEV_WORKSPACE) $(DEV_CONTAINER) fish

all: proto backend frontend wrapper ## Build all components

build: proto ## Parallel build of all components
	@echo "Building all components in parallel..."
	@$(MAKE) --no-print-directory -j3 SKIP_PROTO_DEP=1 backend-bare frontend-bare wrapper-bare

install: build ## Install as a system service (systemd first, rc.local fallback)
	@INSTALL_PREFIX="$(INSTALL_PREFIX)" \
	 INSTALL_BINDIR="$(INSTALL_BINDIR)" \
	 INSTALL_SYSCONFDIR="$(INSTALL_SYSCONFDIR)" \
	 INSTALL_SERVICE_NAME="$(INSTALL_SERVICE_NAME)" \
	 INSTALL_METHOD="$(INSTALL_METHOD)" \
	 INSTALL_ENABLE="$(INSTALL_ENABLE)" \
	 INSTALL_START="$(INSTALL_START)" \
	 ./scripts/install-service.sh install

uninstall: ## Remove the installed system service and installed files
	@INSTALL_PREFIX="$(INSTALL_PREFIX)" \
	 INSTALL_BINDIR="$(INSTALL_BINDIR)" \
	 INSTALL_SYSCONFDIR="$(INSTALL_SYSCONFDIR)" \
	 INSTALL_SERVICE_NAME="$(INSTALL_SERVICE_NAME)" \
	 INSTALL_METHOD="$(INSTALL_METHOD)" \
	 ./scripts/install-service.sh uninstall

backend-bare:
	@echo "Building backend..."
	cd backend/ebpf && go generate && go generate gen_tls.go && go generate gen_cgroup.go && go generate gen_lsm.go
	cd backend && go build $(GO_BUILD_TAGS_ARG) -o agent-ebpf-filter

frontend-bare:
	@echo "Building frontend..."
	cd frontend && bun install && bun run build

wrapper-bare:
	@echo "Building wrapper..."
	cd wrapper && go build -o ../agent-wrapper

cuda: ## Build CUDA acceleration library
	@if [ -x /opt/cuda/bin/nvcc ]; then \
		echo "Building CUDA kernels..."; \
		cd backend/cuda && nvcc -c -Xcompiler -fPIC -o kernels.o kernels.cu && ar rcs libmlcuda.a kernels.o && rm -f kernels.o; \
		echo "CUDA library built (libmlcuda.a)"; \
	else \
		echo "nvcc not found — skipping CUDA build (CPU only)"; \
	fi


predev: ## Install development dependencies in parallel
	@$(MAKE) --no-print-directory -j3 predev-go predev-python predev-frontend
	@$(MAKE) --no-print-directory predev-check
	@echo "Development dependencies are ready."

predev-check: ## Verify development dependencies without installing anything
	@command -v protoc-gen-go >/dev/null || (echo "Missing protoc-gen-go. Run 'make predev' first." && exit 1)
	@command -v node >/dev/null || (echo "Missing node. Install the official Node.js runtime or rebuild/pull the devcontainer image." && exit 1)
	@test -x adapters/python/.venv/bin/python || (echo "Missing adapters/python/.venv. Run 'make predev' first." && exit 1)
	@test -x frontend/node_modules/.bin/pbjs || (echo "Missing frontend/node_modules. Run 'make predev' first." && exit 1)

predev-go:
	@which protoc-gen-go > /dev/null || ( \
		echo "Installing protoc-gen-go into $(GOPATH)/bin..."; \
		mkdir -p "$(GOPATH)/bin" "$(GOPATH)/pkg/mod"; \
		$(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@latest; \
	)

predev-python:
	@if [ ! -x "adapters/python/.venv/bin/python" ]; then \
		echo "Initializing python env..."; \
		cd adapters/python && uv sync; \
	fi

predev-frontend:
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend deps..."; \
		cd frontend && bun install; \
	fi

deps: predev

ifneq ($(SKIP_PREDEV),1)
proto: predev
endif
proto: ## Generate Protocol Buffers code
	@if [ -n "$(SKIP_PREDEV)" ]; then $(MAKE) --no-print-directory proto-check; fi
	@echo "Generating Protocol Buffers code..."
	mkdir -p backend/pb
	mkdir -p adapters/python adapters/js frontend/src/pb
	@set -e; \
		protoc --go_out=backend/pb --go_opt=paths=source_relative -I proto proto/tracker.proto & pid_go=$$!; \
		(cd adapters/python && uv run python -m grpc_tools.protoc -I ../../proto --python_out=. ../../proto/tracker.proto) & pid_py=$$!; \
		(cd frontend && node node_modules/protobufjs-cli/bin/pbjs -t static-module -w commonjs -o ../adapters/js/tracker_pb.js ../proto/tracker.proto && node node_modules/protobufjs-cli/bin/pbjs -t static-module -w es6 -o src/pb/tracker_pb.js ../proto/tracker.proto && node node_modules/protobufjs-cli/bin/pbts -o src/pb/tracker_pb.d.ts src/pb/tracker_pb.js) & pid_js=$$!; \
		for pid in $$pid_go $$pid_py $$pid_js; do wait $$pid; done
	@echo "Proto generation complete."

proto-check:
	@$(MAKE) --no-print-directory predev-check

backend: cuda proto ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate && go generate gen_tls.go && go generate gen_cgroup.go && go generate gen_lsm.go
	cd backend && go build $(GO_BUILD_TAGS_ARG) -o agent-ebpf-filter

ifneq ($(SKIP_PROTO_DEP),1)
wrapper: proto
endif
wrapper: ## Build CLI wrapper
	@echo "Building wrapper..."
	cd wrapper && go build -o ../agent-wrapper

frontend: ## Build Vue3 frontend
	@echo "Building frontend..."
	cd frontend && bun install && bun run build

ebpf-bootstrap: ## Pre-build the backend binary (bootstrap happens automatically on first run)
	@(cd backend/ebpf && go generate && go generate gen_tls.go && go generate gen_cgroup.go && go generate gen_lsm.go)
	@(cd backend && go build $(GO_BUILD_TAGS_ARG) -o agent-ebpf-filter)

ebpf-tls: ## Generate TLS capture eBPF bindings
	@(cd backend/ebpf && go generate gen_tls.go)

ebpf-cgroup: ## Generate cgroup sandbox eBPF bindings
	@(cd backend/ebpf && go generate gen_cgroup.go)

ebpf-lsm: ## Generate BPF LSM enforcement bindings
	@(cd backend/ebpf && go generate gen_lsm.go)

dev: ## Run backend and frontend development server in Zellij (run make predev first)
	@$(MAKE) --no-print-directory SKIP_PREDEV=1 SKIP_PROTO_DEP=1 proto
	@$(MAKE) --no-print-directory SKIP_PREDEV=1 SKIP_PROTO_DEP=1 wrapper
	@./scripts/dev-zellij.sh

dev-backend: ## Run only the backend with self-implemented hot-reload
	@echo "Starting backend dev environment..."
	@./scripts/dev-backend.sh

ml-sweep: ## Run the offline ML benchmark sweep and emit SVG/HTML charts
	@ML_SWEEP_MODE="$(ML_SWEEP_MODE)" ML_SWEEP_MODELS="$(ML_SWEEP_MODELS)" ML_SWEEP_DATASETS="$(ML_SWEEP_DATASETS)" ML_SWEEP_POINTS_PER_PARAM="$(ML_SWEEP_POINTS_PER_PARAM)" ML_SWEEP_WORKERS="$(ML_SWEEP_WORKERS)" ML_SWEEP_RESUME="$(ML_SWEEP_RESUME)" ML_SWEEP_OUTDIR="$(ML_SWEEP_OUTDIR)" ML_SWEEP_REPEATS="$(ML_SWEEP_REPEATS)" ML_SWEEP_STABILITY_TOP="$(ML_SWEEP_STABILITY_TOP)" ./scripts/ml-sweep.sh

ml-presentation: ## Render the PPTX-style HTML presentation from the latest ML sweep report
	@python scripts/render_ml_presentation.py

runtime-benchmark: ## Replay benign/malicious/agentic runtime scenarios and emit a JSON summary
	@./scripts/runtime-replay-benchmark.sh

os-enforcement-smoke: ## Verify live cgroup/connect and BPF LSM blocking against a privileged running backend
	@./scripts/os-enforcement-smoke.sh

os-enforcement-smoke-start: ## Build/start a privileged backend, then run the live OS-level enforcement smoke test
	@OS_SMOKE_START_BACKEND=1 OS_SMOKE_BUILD_BACKEND=1 ./scripts/os-enforcement-smoke.sh

os-enforcement-preflight: ## Check host prerequisites for live OS-level enforcement smoke
	@./scripts/os-enforcement-preflight.sh

os-enforcement-check: ebpf-cgroup ebpf-lsm ## Run rootless static checks for OS-level enforcement objects and smoke script
	@bash -n scripts/os-enforcement-smoke.sh
	@bash -n scripts/os-enforcement-preflight.sh
	@llvm-objdump -h backend/ebpf/agentcgroupsandbox_bpfel.o | rg -q 'cgroup/connect4'
	@llvm-objdump -h backend/ebpf/agentcgroupsandbox_bpfel.o | rg -q 'cgroup/connect6'
	@llvm-objdump -h backend/ebpf/agentcgroupsandbox_bpfel.o | rg -q 'cgroup/sendmsg4'
	@llvm-objdump -h backend/ebpf/agentcgroupsandbox_bpfel.o | rg -q 'cgroup/sendmsg6'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/bprm_check_security'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/file_open'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/file_permission'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/mmap_file'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/file_mprotect'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_setattr'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_create'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_link'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_unlink'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_symlink'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_mkdir'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_rmdir'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_mknod'
	@llvm-objdump -h backend/ebpf/agentlsmenforcer_bpfel.o | rg -q 'lsm/inode_rename'
	@cd backend && go test ./... -run 'Test(CgroupSandboxObjectSections|CgroupSandboxPolicySourceUsesHostOrderKeys|CgroupSandboxAttachPathValidation|CgroupSandboxPortValidation|CgroupSandboxIPValidation|OSEnforcementMutationHandlersRejectInvalidInputBeforeLoad|LsmEnforcerObjectSections|LsmPolicyKeys|LsmPolicySourceUsesCurrentHookArguments|OSSmokeScriptExists|OSPreflightScriptExists|OSPolicyMapPinsAreRestrictive|OSEnforcementStartsWithoutDefaultBlockEntries|OSEnforcementMutationRoutesArePolicyGated|OSEnforcementStatusRoutesRequireAuth|OSSecurityDocsDescribeCurrentKernelEnforcement|OSFrontendSecuritySurfaceWiresSandboxEndpoints|PinnedOSEnforcementPolicyIsPreservedOnReuseFailure|OSEnforcementAttachFailureCleansPartialPins|OSEnforcementUnblockIgnoresMissingMapKeys|OSEnforcementStatusUsesRuntimeSnapshots|CgroupPIDResolutionHelpers)' -count=1

test: ## Run all tests (Go backend)
	@echo "Running Go tests..."
	cd backend && go test $(GO_BUILD_TAGS_ARG) -race -count=1 -timeout 120s ./...

dev-frontend: ## Run only the frontend development server
	@echo "Starting frontend dev environment..."
	@./scripts/dev-frontend.sh


run: all ebpf-bootstrap ## Build and run in production mode
	@echo "Running production build..."
	@./backend/agent-ebpf-filter

run-backend: backend ## Build and run only the backend
	@./backend/agent-ebpf-filter

run-frontend: ## Run only the frontend development server
	cd frontend && bun run dev

clean: ## Clean build artifacts
	@rm -f backend/agent-ebpf-filter; \
	 rm -f agent-wrapper; \
	 rm -f backend/.port; \
	 rm -rf frontend/dist; \
	 rm -rf adapters/python/.venv; \
	 rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go; \
	 rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o; \
	 rm -f backend/ebpf/agenttlscapture_x86_bpfel.go backend/ebpf/agenttlscapture_x86_bpfeb.go; \
	 rm -f backend/ebpf/agenttlscapture_x86_bpfel.o backend/ebpf/agenttlscapture_x86_bpfeb.o; \
	 rm -f backend/ebpf/agentcgroupsandbox_bpfel.go backend/ebpf/agentcgroupsandbox_bpfeb.go; \
	 rm -f backend/ebpf/agentcgroupsandbox_bpfel.o backend/ebpf/agentcgroupsandbox_bpfeb.o; \
	 rm -f backend/ebpf/agentlsmenforcer_bpfel.go backend/ebpf/agentlsmenforcer_bpfeb.go; \
	 rm -f backend/ebpf/agentlsmenforcer_bpfel.o backend/ebpf/agentlsmenforcer_bpfeb.o; \
	 rm -rf backend/pb; \
	 rm -rf frontend/src/pb;

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed -e 's/:.*## /: /'
