# Get Go binaries path
GOPATH ?= $(shell go env GOPATH)
export PATH := $(PATH):$(GOPATH)/bin

.PHONY: all backend frontend clean proto help dev run deps

all: proto backend frontend ## Build both backend and frontend

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

backend: proto ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate
	cd backend && go build -o agent-ebpf-filter

frontend: ## Build Vue3 frontend
	@echo "Building frontend..."
	cd frontend && bun install && bun run build

dev: proto ## Run both backend and frontend development server (no full build)
	@echo "Stopping any existing backends..."
	@-sudo pkill -f agent-ebpf-filter || true
	@echo "Starting dev environment..."
	cd backend/ebpf && go generate
	cd backend && go run main.go & \
	cd frontend && bun run dev

run: all ## Build and run in production mode
	@echo "Stopping any existing backends..."
	@-sudo pkill -f agent-ebpf-filter || true
	@echo "Running production build..."
	@./backend/agent-ebpf-filter

run-backend: backend ## Build and run only the backend
	@-sudo pkill -f agent-ebpf-filter || true
	@./backend/agent-ebpf-filter

run-frontend: ## Run only the frontend development server
	cd frontend && bun run dev

clean: ## Clean build artifacts
	rm -f backend/agent-ebpf-filter
	rm -f backend/.port
	rm -rf frontend/dist
	rm -rf adapters/python/.venv
	rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go
	rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o
	rm -rf backend/pb
	rm -rf frontend/src/pb

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed -e 's/:.*## /: /'
