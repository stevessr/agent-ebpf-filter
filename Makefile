# Get Go binaries path
GOPATH ?= $(shell go env GOPATH)
export PATH := $(PATH):$(GOPATH)/bin

CONTAINER_CLI ?= $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
DEV_IMAGE ?= agent-ebpf-filiter-dev
DEV_CONTAINER ?= agent-ebpf-filiter-dev
DEV_WORKSPACE ?= /workspaces/agent-ebpf-filiter
DEVCONTAINER_GO_VERSION ?= 1.26.2
DEVCONTAINER_USER_UID ?= 1001
DEVCONTAINER_USER_GID ?= 1001

.PHONY: all backend frontend wrapper clean proto proto-check help predev predev-go predev-python predev-frontend dev run deps ebpf-bootstrap ebpf-tls cuda ml-sweep ml-presentation runtime-benchmark test build docker exec


docker: ## Build the privileged devcontainer image; use 'make docker clean' to prune build cache
	@test -n "$(CONTAINER_CLI)" || (echo "Missing docker or podman CLI." && exit 1)
	@if printf '%s\n' "$(MAKECMDGOALS)" | grep -qw clean; then \
		echo "Removing $(CONTAINER_CLI) build cache..."; \
		$(CONTAINER_CLI) builder prune -f || $(CONTAINER_CLI) system prune -f; \
	else \
		$(CONTAINER_CLI) build \
			-f .devcontainer/Dockerfile \
			--build-arg GO_VERSION=$(DEVCONTAINER_GO_VERSION) \
			--build-arg USER_UID=$(DEVCONTAINER_USER_UID) \
			--build-arg USER_GID=$(DEVCONTAINER_USER_GID) \
			-t $(DEV_IMAGE) \
			.devcontainer; \
	fi

exec: ## Start or attach to the mounted devcontainer shell
	@test -n "$(CONTAINER_CLI)" || (echo "Missing docker or podman CLI." && exit 1)
	@$(CONTAINER_CLI) image inspect $(DEV_IMAGE) >/dev/null 2>&1 || $(MAKE) --no-print-directory docker
	@if $(CONTAINER_CLI) container inspect $(DEV_CONTAINER) >/dev/null 2>&1; then \
		if [ "$$($(CONTAINER_CLI) inspect -f '{{.State.Running}}' $(DEV_CONTAINER))" != "true" ]; then \
			echo "Starting existing container $(DEV_CONTAINER)..."; \
			$(CONTAINER_CLI) start $(DEV_CONTAINER) >/dev/null; \
		fi; \
	else \
		echo "Creating container $(DEV_CONTAINER) from $(DEV_IMAGE)..."; \
		$(CONTAINER_CLI) run -dit \
			--name $(DEV_CONTAINER) \
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
			-v /sys/kernel/debug:/sys/kernel/debug \
			-v /sys/fs/bpf:/sys/fs/bpf \
			-v /lib/modules:/lib/modules:ro \
			-w $(DEV_WORKSPACE) \
			$(DEV_IMAGE) fish >/dev/null; \
	fi
	$(CONTAINER_CLI) exec -it -w $(DEV_WORKSPACE) $(DEV_CONTAINER) fish

all: proto backend frontend wrapper ## Build all components

build: proto ## Parallel build of all components
	@echo "Building all components in parallel..."
	@$(MAKE) --no-print-directory -j3 SKIP_PROTO_DEP=1 backend-bare frontend-bare wrapper-bare

backend-bare:
	@echo "Building backend..."
	cd backend/ebpf && go generate && go generate gen_tls.go
	cd backend && go build -o agent-ebpf-filter

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
	@echo "Development dependencies are ready."

predev-go:
	@which protoc-gen-go > /dev/null || (echo "Installing protoc-gen-go..." && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest)

predev-python:
	@if [ ! -d "adapters/python/.venv" ]; then \
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
		(cd frontend && bunx pbjs -t static-module -w commonjs -o ../adapters/js/tracker_pb.js ../proto/tracker.proto && bunx pbjs -t static-module -w es6 -o src/pb/tracker_pb.js ../proto/tracker.proto && bunx pbts -o src/pb/tracker_pb.d.ts src/pb/tracker_pb.js) & pid_js=$$!; \
		for pid in $$pid_go $$pid_py $$pid_js; do wait $$pid; done
	@echo "Proto generation complete."

proto-check:
	@command -v protoc-gen-go >/dev/null || (echo "Missing protoc-gen-go. Run 'make predev' first." && exit 1)
	@test -d adapters/python/.venv || (echo "Missing adapters/python/.venv. Run 'make predev' first." && exit 1)
	@test -d frontend/node_modules || (echo "Missing frontend/node_modules. Run 'make predev' first." && exit 1)

backend: cuda proto ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate && go generate gen_tls.go
	cd backend && go build -o agent-ebpf-filter

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
	@(cd backend/ebpf && go generate && go generate gen_tls.go)
	@(cd backend && go build -o agent-ebpf-filter)

ebpf-tls: ## Generate TLS capture eBPF bindings
	@(cd backend/ebpf && go generate gen_tls.go)

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

test: ## Run all tests (Go backend)
	@echo "Running Go tests..."
	cd backend && go test -race -count=1 -timeout 120s ./...

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
	@if printf '%s\n' "$(MAKECMDGOALS)" | grep -qw docker; then \
		echo "Docker build cache cleaned by docker target."; \
	else \
		rm -f backend/agent-ebpf-filter; \
		rm -f agent-wrapper; \
		rm -f backend/.port; \
		rm -rf frontend/dist; \
		rm -rf adapters/python/.venv; \
		rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go; \
		rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o; \
		rm -f backend/ebpf/agenttlscapture_x86_bpfel.go backend/ebpf/agenttlscapture_x86_bpfeb.go; \
		rm -f backend/ebpf/agenttlscapture_x86_bpfel.o backend/ebpf/agenttlscapture_x86_bpfeb.o; \
		rm -rf backend/pb; \
		rm -rf frontend/src/pb; \
	fi

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed -e 's/:.*## /: /'
