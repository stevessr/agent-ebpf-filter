# Kubernetes Integration

This repository includes a minimal Kubernetes deployment under
`deploy/kubernetes/`. It runs one node-local backend per node with a DaemonSet
and exposes the authenticated external API through a ClusterIP Service.

The shape follows Kubernetes' node-agent pattern: DaemonSets are meant for
workloads that must run on every node, and Services provide stable in-cluster
network access to Pods.

## Prerequisites

- Linux worker nodes with eBPF, BTF, bpffs, cgroup v2, and BPF LSM support when
  LSM enforcement is required.
- A container image that contains the built `agent-ebpf-filter` backend,
  frontend assets, and `agent-wrapper`.
- Cluster policy that allows this privileged node agent. The manifest needs
  `privileged: true`, `hostPID: true`, `hostNetwork: true`, and hostPath mounts
  for `/sys/fs/bpf`, `/sys/fs/cgroup`, and runtime state.
- A strong runtime API token stored in the `agent-ebpf-filter-token` Secret.

## Install

Edit the Secret and image first:

```bash
kubectl apply -f deploy/kubernetes/agent-ebpf-filter.yaml
kubectl -n agent-ebpf-filter create secret generic agent-ebpf-filter-token \
  --from-literal=AGENT_API_KEY='<replace-with-random-token>' \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n agent-ebpf-filter set image daemonset/agent-ebpf-filter \
  agent-ebpf-filter=ghcr.io/<owner>/<repo>:<tag>
```

Or use kustomize:

```bash
kubectl apply -k deploy/kubernetes
```

Check rollout and health:

```bash
kubectl -n agent-ebpf-filter rollout status daemonset/agent-ebpf-filter
kubectl -n agent-ebpf-filter get pods -o wide
kubectl -n agent-ebpf-filter port-forward svc/agent-ebpf-filter 8080:8080
curl -H "X-API-KEY: $AGENT_API_KEY" http://127.0.0.1:8080/api/v1/health
```

## API access from inside the cluster

Use the ClusterIP Service DNS name:

```bash
curl -H "X-API-KEY: $AGENT_API_KEY" \
  http://agent-ebpf-filter.agent-ebpf-filter.svc.cluster.local:8080/api/v1/network/flows
```

The default Service is `ClusterIP`, so it is reachable from within the cluster.
If you need outside-cluster access, expose it through your normal ingress,
Gateway, VPN, or a carefully restricted `NodePort` / `LoadBalancer`. Keep the
backend token mandatory and avoid publishing the Service directly to the public
internet.

## Optional 80/443 domain forwarding

The DaemonSet uses `hostNetwork: true`, so enabling
`domainForwardProxy.enabled` in `/config/runtime` can bind the node's port `80`
and `443` directly. The manifest declares `forward-http` and `forward-https`
container/Service ports, but the listeners stay closed until the runtime setting
is enabled.

Mount certificate material with your normal Secret flow, for example under
`/etc/agent-ebpf-filter/certs`, then set `domainForwardProxy.certFile` and
`domainForwardProxy.keyFile` or per-route `certFile` / `keyFile`. If cluster or
lab DNS redirects all domains back to the node, set
`domainForwardProxy.dnsResolver` (for example `1.1.1.1:53`) or explicit
per-host upstreams so proxied outbound requests do not loop back to the same
listener.

## Node-specific behavior

The DaemonSet runs one backend per node. A ClusterIP Service may route a request
to any ready Pod, so use one of these for node-specific diagnostics:

```bash
kubectl -n agent-ebpf-filter port-forward pod/<pod-on-target-node> 8080:8080
kubectl -n agent-ebpf-filter exec -it pod/<pod-on-target-node> -- /bin/sh
```

The existing master/slave cluster mode can still be used for aggregation, but it
is not enabled by the base manifest. Configure `AGENT_CLUSTER_MASTER_URL`,
`AGENT_CLUSTER_ACCOUNT`, `AGENT_CLUSTER_PASSWORD`, and node identity variables
when you want a dedicated master backend to forward requests to node agents.

## Security notes

- The DaemonSet is privileged because it loads/pins eBPF programs and inspects
  host runtime state. Treat it as a node-level security component.
- Do not grant Kubernetes RBAC permissions unless future code actually reads the
  Kubernetes API. The base ServiceAccount intentionally has no ClusterRole.
- Use a strong `AGENT_API_KEY`; it protects `/api/v1/**`, `/config/**`,
  `/system/**`, `/sandbox/**`, `/metrics`, WebSockets, MCP, and registration
  routes in release mode.
- Keep policy mutations disabled until needed. Enable `policyManagementEnabled`
  only when an external controller should add/remove cgroup or BPF LSM blocks.
- Keep the domain forwarder disabled unless the node is intended to terminate
  HTTP/HTTPS for those domains. Proxied traffic itself is a public data plane;
  only `/config/runtime` and `/system/domain-forward/status` are token-protected.
- NetworkPolicy support depends on the CNI plugin. If your cluster supports it,
  restrict ingress to the namespaces/controllers that must call the API.

## Useful external API calls

```bash
# Service / kernel bootstrap health
curl -H "X-API-KEY: $AGENT_API_KEY" \
  http://127.0.0.1:8080/api/v1/health

# Recent events
curl -H "X-API-KEY: $AGENT_API_KEY" \
  "http://127.0.0.1:8080/api/v1/events/recent?limit=50"

# Network flows
curl -H "X-API-KEY: $AGENT_API_KEY" \
  "http://127.0.0.1:8080/api/v1/network/flows?filter=process:kubectl"

# BPF LSM status
curl -H "X-API-KEY: $AGENT_API_KEY" \
  http://127.0.0.1:8080/api/v1/sandbox/lsm/status
```

See `docs/external-api.md` for the stable `/api/v1` route list.

---

## - [External API](../integrations/external-api.md)
- [部署与安装](deployment.md)
- [开发容器](devcontainer.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [MCP、External API 与 OTLP](../integrations/mcp-external-otlp.md)

