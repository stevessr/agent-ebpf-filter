# Initial ABI notes

The C backend currently emits normal libbpf map definitions and probe sections. `kprobe` and `tracepoint` decorators encode their attach points in section names. `uprobe` emits a generic `SEC("uprobe")` program and preserves the userspace symbol in the JSON attach manifest; the production probe manager can consume that metadata when loader integration is added.

The initial `bpf.arg(ctx, N)` lowering supports argument registers 1 through 5 and relies on the target architecture macro supplied to clang (for example `-D__TARGET_ARCH_x86`).
