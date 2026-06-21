# Documentation Index

This index helps you quickly find the right documentation for your needs.

---

## 📖 By Role

### I'm a New Developer

Start here to understand the project and get up and running:

1. **[What is Agent eBPF Filter?](docs/guide/what-is-agent-ebpf-filter.md)** — Project overview and core concepts
2. **[Quick Start](docs/guide/quick-start.md)** — Get the project running in minutes
3. **[AGENTS.md](AGENTS.md)** — Developer workflow guide
4. **[Project Structure](docs/guide/reading-paths.md)** — Understanding the codebase
5. **[Architecture Overview](docs/architecture/overview.md)** — System design

### I'm Integrating an AI Agent

Learn how to connect your agent to the observability system:

1. **[Agent Registration Guide](agents.md)** — How agents are tracked
2. **[Python Adapter](adapters/python/README.md)** — Use with Python agents
3. **[Node.js Adapter](adapters/js/README.md)** — Use with Node.js agents
4. **[Native Hooks](docs/integrations/native-hooks.md)** — Hook into Claude Code, Codex, etc.
5. **[MCP Integration](docs/integrations/mcp.md)** — Expose agent controls via MCP

### I'm a Security Reviewer

Focus on the security model and enforcement mechanisms:

1. **[Security Model](docs/security/model.md)** — Overall security architecture
2. **[Threat Model](docs/security/threat-model.md)** — Threat analysis and mitigations
3. **[Policy Semantics](docs/security/policy-semantics.md)** — How policies work
4. **[Runtime Gates & Auth](docs/security/runtime-gates-auth.md)** — Feature gates and authentication
5. **[Data Redaction](backend/redaction/README.md)** — Sensitive data protection

### I'm Deploying to Production

Deployment, operations, and monitoring:

1. **[Build & Run](docs/operations/build-and-run.md)** — Building and running the system
2. **[Deployment Guide](docs/operations/deployment.md)** — System service installation
3. **[Kubernetes](docs/operations/kubernetes.md)** — Deploy as DaemonSet
4. **[External API](docs/integrations/external-api.md)** — API for automation
5. **[OTLP Export](docs/integrations/otel-export.md)** — Observability integration

### I'm an AI Coding Assistant

Working on this codebase with Claude Code or similar tools:

1. **[AGENTS.md](AGENTS.md)** — Developer & coding agent guide
2. **[Project Structure Skill](.claude/skills/project-structure/SKILL.md)** — Navigate the codebase
3. **[Configure Security Skill](.claude/skills/configure-security/SKILL.md)** — Manage policies
4. **[Code Entry Points](docs/reference/code-entrypoints.md)** — Where to start reading

---

## 📂 By Component

### Backend (Go + eBPF)

| Document | Description |
|----------|-------------|
| [Backend README](backend/README.md) | Backend architecture and internals |
| [Runtime Startup](docs/backend/runtime-startup.md) | Startup sequence and initialization |
| [Routes & API](docs/backend/routes-api.md) | HTTP/WebSocket API endpoints |
| [Event Pipeline](docs/backend/event-pipeline.md) | Event collection and processing |
| [eBPF Programs](docs/backend/ebpf-programs.md) | Kernel tracing, cgroup, and LSM |
| [ML Models](docs/backend/ml-models-summary.md) | Machine learning classification |

### Frontend (Vue 3)

| Document | Description |
|----------|-------------|
| [Frontend README](frontend/README.md) | Frontend structure and routing |
| [Workbench Overview](docs/frontend/workbench.md) | Dashboard and UI components |
| [Dashboard](docs/frontend/dashboard.md) | Real-time event stream view |
| [Network View](docs/frontend/network.md) | Network flow visualization |
| [Execution Graph](docs/frontend/execution-graph.md) | Process topology view |

### Wrapper & Adapters

| Document | Description |
|----------|-------------|
| [Wrapper README](wrapper/README.md) | Command interceptor protocol |
| [Python Adapter](adapters/python/README.md) | Python PID registration |
| [Node.js Adapter](adapters/js/README.md) | Node.js PID registration |
| [Agent Registration](agents.md) | How tracking works |

---

## 🎯 By Task

### Setting Up Development Environment

1. [Quick Start](docs/guide/quick-start.md)
2. [Development Setup](AGENTS.md#development-setup)
3. [Docker Devcontainer](docs/operations/devcontainer.md)

### Adding a New Feature

1. [Code Entry Points](docs/reference/code-entrypoints.md)
2. [Making Changes](AGENTS.md#making-changes)
3. [Testing & Validation](AGENTS.md#testing--validation)

### Configuring Security Policies

1. [Policy Semantics](docs/security/policy-semantics.md)
2. [Runtime Configuration](docs/backend/runtime-settings-features.md)
3. [OS Enforcement](docs/backend/ebpf-os-enforcement.md)

### Debugging Issues

1. [Troubleshooting](docs/operations/troubleshooting.md)
2. [Debugging Tips](AGENTS.md#debugging-tips)
3. [Common Issues](docs/operations/troubleshooting.md)

### Integrating with External Systems

1. [External API](docs/integrations/external-api.md)
2. [MCP, External API & OTLP](docs/integrations/mcp-external-otlp.md)
3. [OTLP Export](docs/integrations/otel-export.md)

---

## 🔍 By Topic

### eBPF & Kernel

- [eBPF Programs](docs/backend/ebpf-programs.md)
- [OS-Level Enforcement](docs/backend/ebpf-os-enforcement.md)
- [Cgroup Network Blocking](docs/backend/cgroup-sandbox.md)
- [BPF LSM File Blocking](docs/backend/lsm-enforcer.md)
- [Kernel ML Module](docs/backend/kernel-ml-implementation.md)

### Security & Privacy

- [Security Model](docs/security/model.md)
- [Threat Model](docs/security/threat-model.md)
- [Data Redaction](backend/redaction/README.md)
- [Runtime Gates](docs/security/runtime-gates-auth.md)
- [TLS Capture](docs/backend/TLS_QUICKSTART.md) (diagnostic only)

### AI Agent Integration

- [Agent Registration](agents.md)
- [Native Hooks](docs/integrations/native-hooks.md)
- [Wrapper Protocol](wrapper/README.md)
- [MCP Tools](docs/integrations/mcp.md)
- [AgentSight Compatibility](docs/integrations/agentsight.md)

### Machine Learning

- [ML Models Summary](docs/backend/ml-models-summary.md)
- [ML Models Complete Guide](docs/backend/ml-models-complete-guide.md)
- [ML Experiments](docs/backend/ml-experiments.md)
- [Kernel ML Implementation](docs/backend/kernel-ml-implementation.md)

### Deployment & Operations

- [Build & Run](docs/operations/build-and-run.md)
- [Deployment](docs/operations/deployment.md)
- [Kubernetes](docs/operations/kubernetes.md)
- [Validation, Testing & Benchmark](docs/operations/verification-benchmark.md)
- [Troubleshooting](docs/operations/troubleshooting.md)

---

## 📚 Complete Documentation Structure

```
agent-ebpf-filter/
├── README.md                           # Main project overview
├── README_cn.md                        # Chinese version
├── AGENTS.md                           # Developer & coding agent guide
├── agents.md                           # Agent registration runtime guide
├── CLAUDE.md -> AGENTS.md              # Symlink for Claude Code
│
├── docs/                               # Documentation site
│   ├── index.md                        # Documentation home
│   │
│   ├── guide/                          # Getting Started
│   │   ├── what-is-agent-ebpf-filter.md
│   │   ├── quick-start.md
│   │   ├── capabilities.md
│   │   ├── reading-paths.md
│   │   └── diagrams-and-examples.md
│   │
│   ├── architecture/                   # System Design
│   │   ├── overview.md
│   │   ├── data-flow.md
│   │   ├── components.md
│   │   └── runtime-boundaries.md
│   │
│   ├── backend/                        # Backend Documentation
│   │   ├── runtime-startup.md
│   │   ├── routes-api.md
│   │   ├── event-pipeline.md
│   │   ├── ebpf-programs.md
│   │   ├── ebpf-os-enforcement.md
│   │   ├── cgroup-sandbox.md
│   │   ├── lsm-enforcer.md
│   │   ├── runtime-settings-features.md
│   │   ├── TLS_QUICKSTART.md
│   │   ├── ml-models-summary.md
│   │   ├── ml-models-complete-guide.md
│   │   ├── ml-experiments.md
│   │   ├── ml-benchmark-report.md
│   │   └── kernel-ml-implementation.md
│   │
│   ├── frontend/                       # Frontend Documentation
│   │   ├── workbench.md
│   │   ├── routes-and-pages.md
│   │   ├── components-composables.md
│   │   ├── build-feature-flags.md
│   │   ├── dashboard.md
│   │   ├── network.md
│   │   ├── execution-graph.md
│   │   └── configuration.md
│   │
│   ├── security/                       # Security Documentation
│   │   ├── model.md
│   │   ├── policy-semantics.md
│   │   ├── threat-model.md
│   │   ├── runtime-gates-auth.md
│   │   ├── redaction-privacy.md
│   │   ├── sanitization.md
│   │   └── sanitization-user-guide.md
│   │
│   ├── integrations/                   # Integration Guides
│   │   ├── agents.md
│   │   ├── wrapper.md
│   │   ├── native-hooks.md
│   │   ├── mcp-external-otlp.md
│   │   ├── otel-export.md
│   │   └── external-api.md
│   │
│   ├── operations/                     # Operations & Deployment
│   │   ├── build-and-run.md
│   │   ├── deployment.md
│   │   ├── devcontainer.md
│   │   ├── kubernetes.md
│   │   ├── verification-benchmark.md
│   │   ├── runtime-replay-benchmark.md
│   │   └── troubleshooting.md
│   │
│   ├── delivery/                       # Competition Delivery
│   │   ├── competition-defense.md
│   │   ├── demo-script.md
│   │   ├── compliance.md
│   │   └── evaluation.md
│   │
│   └── reference/                      # Reference Materials
│       ├── documentation-map.md
│       ├── documentation-audit.md
│       ├── technical-depth.md
│       ├── implementation-patterns.md
│       ├── technical-comparison.md
│       ├── code-entrypoints.md
│       ├── generated-files.md
│       ├── performance-models.md
│       ├── external-resources.md
│       ├── vitepress-plugins.md
│       ├── agentsight-acknowledgment.md
│       ├── maintenance-checklists.md
│       ├── DEV_DOCS_INDEX.md
│       ├── project-roadmap.md
│       ├── project-structure-deep-dive.md
│       └── codebase-implementation-map.md
│
├── backend/                            # Backend code & docs
│   ├── README.md
│   ├── redaction/
│   │   └── README.md
│   └── docs/
│       └── ssl_hook_key_removal.md
│
├── frontend/                           # Frontend code
│   └── README.md
│
├── wrapper/                            # Wrapper code
│   └── README.md
│
├── adapters/                           # Language adapters
│   ├── python/
│   │   └── README.md
│   └── js/
│       └── README.md
│
└── .claude/skills/                     # Claude Code skills
    ├── project-structure/
    │   └── SKILL.md
    ├── configure-security/
    │   └── SKILL.md
    ├── analyze-network/
    │   └── SKILL.md
    └── monitor-process/
        └── SKILL.md
```

---

## 🎓 Learning Paths

### Path 1: Understanding the System (1-2 hours)

1. Read [What is Agent eBPF Filter?](docs/guide/what-is-agent-ebpf-filter.md)
2. Review [Architecture Overview](docs/architecture/overview.md)
3. Explore [Capabilities](docs/guide/capabilities.md)
4. Check [Diagrams & Examples](docs/guide/diagrams-and-examples.md)

### Path 2: Getting Hands-On (2-3 hours)

1. Follow [Quick Start](docs/guide/quick-start.md)
2. Read [Agent Registration](agents.md)
3. Try [Python Adapter](adapters/python/README.md)
4. Test [Wrapper](wrapper/README.md)

### Path 3: Deep Dive into Code (4-6 hours)

1. Read [AGENTS.md](AGENTS.md) developer guide
2. Study [Code Entry Points](docs/reference/code-entrypoints.md)
3. Explore [Backend Internals](backend/README.md)
4. Review [eBPF Programs](docs/backend/ebpf-programs.md)
5. Understand [Event Pipeline](docs/backend/event-pipeline.md)

### Path 4: Security & Deployment (3-4 hours)

1. Study [Security Model](docs/security/model.md)
2. Read [Policy Semantics](docs/security/policy-semantics.md)
3. Review [Runtime Gates](docs/security/runtime-gates-auth.md)
4. Follow [Deployment Guide](docs/operations/deployment.md)
5. Check [Validation](docs/operations/validation.md)

---

## 🆘 Need Help?

- **Can't find what you're looking for?** Try searching with `grep -r "keyword" docs/`
- **Code questions?** Check [AGENTS.md](AGENTS.md) and [Code Entry Points](docs/reference/code-entrypoints.md)
- **Integration issues?** See [Integrations](docs/integrations/) section
- **Deployment problems?** Check [Troubleshooting](docs/operations/troubleshooting.md)

---

## 🔗 Quick Links

| Link | Purpose |
|------|---------|
| [Main README](README.md) | Project overview |
| [中文 README](README_cn.md) | Chinese project overview |
| [AGENTS.md](AGENTS.md) | Developer workflow guide |
| [Documentation Site](docs/index.md) | Full documentation |
| [Security Model](docs/security/model.md) | Security architecture |
| [API Reference](docs/integrations/external-api.md) | External API docs |

---

**Last Updated:** 2025-01-XX
**Documentation Version:** 1.0

---

*This index is maintained manually. If you add new documentation, please update this file.*
