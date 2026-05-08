# Get Go binaries path
GOPATH ?= $(shell go env GOPATH)
export PATH := $(PATH):$(GOPATH)/bin

.PHONY: all backend frontend wrapper clean proto help dev run deps ebpf-bootstrap cuda ml-sweep ml-presentation

all: proto backend frontend wrapper ## Build all components

cuda: ## Build CUDA acceleration library
	@if [ -x /opt/cuda/bin/nvcc ]; then \
		echo "Building CUDA kernels..."; \
		cd backend/cuda && nvcc -c -Xcompiler -fPIC -o kernels.o kernels.cu && ar rcs libmlcuda.a kernels.o && rm -f kernels.o; \
		echo "CUDA library built (libmlcuda.a)"; \
	else \
		echo "nvcc not found — skipping CUDA build (CPU only)"; \
	fi


deps: ## Ensure Go and Python build dependencies are installed
	@echo "Checking dependencies..."
	@which protoc-gen-go > /dev/null || (echo "Installing protoc-gen-go..." && go install google.golang.org/protobuf/cmd/protoc-gen-go@latest)
	@if [ ! -d "adapters/python/.venv" ]; then \
		echo "Initializing python env..."; \
		cd adapters/python && uv sync; \
	fi
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend deps..."; \
		cd frontend && bun install; \
	fi

proto: deps ## Generate Protocol Buffers code
	@echo "Generating Protocol Buffers code..."
	mkdir -p backend/pb
	protoc --go_out=backend/pb --go_opt=paths=source_relative \
		-I proto proto/tracker.proto
	mkdir -p adapters/python
	cd adapters/python && uv run python -m grpc_tools.protoc -I ../../proto --python_out=. ../../proto/tracker.proto
	mkdir -p adapters/js
	cd frontend && bunx pbjs -t static-module -w commonjs -o ../adapters/js/tracker_pb.js ../proto/tracker.proto
	mkdir -p frontend/src/pb
	cd frontend && bunx pbjs -t static-module -w es6 -o src/pb/tracker_pb.js ../proto/tracker.proto
	cd frontend && bunx pbts -o src/pb/tracker_pb.d.ts src/pb/tracker_pb.js
	@echo "Proto generation complete."

backend: cuda proto ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate
	cd backend && go build -o agent-ebpf-filter

wrapper: proto ## Build CLI wrapper
	@echo "Building wrapper..."
	cd wrapper && go build -o ../agent-wrapper

frontend: ## Build Vue3 frontend
	@echo "Building frontend..."
	cd frontend && bun install && bun run build

ebpf-bootstrap: ## Pre-build the backend binary (bootstrap happens automatically on first run)
	@(cd backend/ebpf && go generate)
	@(cd backend && go build -o agent-ebpf-filter)

dev: proto wrapper ## Run both backend and frontend development server (nx TUI separated)
	@if [ ! -d "node_modules" ]; then bun install; fi
	@./node_modules/.bin/nx run-many --targets=dev-backend,dev-frontend --parallel=2

dev-backend: ## Run only the backend with self-implemented hot-reload
	@echo "Starting backend dev environment..."
	@./scripts/dev-backend.sh

ml-sweep: ## Run the offline ML benchmark sweep and emit SVG/HTML charts
	@ML_SWEEP_MODE="$(ML_SWEEP_MODE)" ML_SWEEP_MODELS="$(ML_SWEEP_MODELS)" ML_SWEEP_DATASETS="$(ML_SWEEP_DATASETS)" ML_SWEEP_POINTS_PER_PARAM="$(ML_SWEEP_POINTS_PER_PARAM)" ML_SWEEP_WORKERS="$(ML_SWEEP_WORKERS)" ML_SWEEP_RESUME="$(ML_SWEEP_RESUME)" ML_SWEEP_OUTDIR="$(ML_SWEEP_OUTDIR)" ML_SWEEP_REPEATS="$(ML_SWEEP_REPEATS)" ML_SWEEP_STABILITY_TOP="$(ML_SWEEP_STABILITY_TOP)" ./scripts/ml-sweep.sh

ml-presentation: ## Render the PPTX-style HTML presentation from the latest ML sweep report
	@python scripts/render_ml_presentation.py

dev-frontend: ## Run only the frontend development server
	@echo "Starting frontend dev environment..."
	@cd frontend && bun run dev


run: all ebpf-bootstrap ## Build and run in production mode
	@echo "Running production build..."
	@./backend/agent-ebpf-filter

run-backend: backend ## Build and run only the backend
	@./backend/agent-ebpf-filter

run-frontend: ## Run only the frontend development server
	cd frontend && bun run dev

clean: ## Clean build artifacts
	rm -f backend/agent-ebpf-filter
	rm -f agent-wrapper
	rm -f backend/.port
	rm -rf frontend/dist
	rm -rf adapters/python/.venv
	rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go
	rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o
	rm -rf backend/pb
	rm -rf frontend/src/pb

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed -e 's/:.*## /: /'
