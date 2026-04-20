.PHONY: all backend frontend clean help

all: backend frontend ## Build both backend and frontend

backend: ## Build Go backend and compile eBPF
	@echo "Building backend..."
	cd backend/ebpf && go generate
	cd backend && go build -o agent-ebpf-filter

frontend: ## Build Vue3 frontend
	@echo "Building frontend..."
	cd frontend && bun install && bun run build

run: all ## Build and run both backend and frontend concurrently
	@echo "Starting backend (sudo required for eBPF)..."
	@sudo ./backend/agent-ebpf-filter & \
	echo "Starting frontend..." && \
	cd frontend && bun run dev

run-backend: backend ## Build and run only the backend (sudo required)
	@sudo ./backend/agent-ebpf-filter

run-frontend: ## Run only the frontend development server
	cd frontend && bun run dev

clean: ## Clean build artifacts
	rm -f backend/agent-ebpf-filter
	rm -rf frontend/dist
	rm -f backend/ebpf/agenttracker_bpfel.go backend/ebpf/agenttracker_bpfeb.go
	rm -f backend/ebpf/agenttracker_bpfel.o backend/ebpf/agenttracker_bpfeb.o

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed -e 's/:.*## /: /'
