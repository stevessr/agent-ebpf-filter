.PHONY: all backend frontend clean help

all: backend frontend ## Build both backend and frontend

backend: ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate
	cd backend && go build -o agent-ebpf-filter

frontend: ## Build Vue3 frontend
	@echo "Building frontend..."
	cd frontend && bun install && bun run build

dev: all ## Run both backend and frontend development server concurrently
	@echo "Starting backend..."
	@./backend/agent-ebpf-filter & \
	echo "Starting frontend dev server..." && \
	cd frontend && bun run dev

run: all ## Build and run in production mode (Backend serves the frontend)
	@echo "Running production build..."
	@./backend/agent-ebpf-filter

run-backend: backend ## Build and run only the backend
	@./backend/agent-ebpf-filter

run-frontend: ## Run only the frontend development server
	cd frontend && bun run dev

clean: ## Clean build artifacts
	rm -f backend/agent-ebpf-filter
	rm -rf frontend/dist
	rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go
	rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed -e 's/:.*## /: /'
