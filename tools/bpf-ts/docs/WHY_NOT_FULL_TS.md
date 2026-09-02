# Why not full TypeScript?

Kernel eBPF has no JavaScript runtime, garbage collector, promises, exceptions or unconstrained heap. bpf-ts therefore treats TypeScript syntax as a familiar surface language, not as JavaScript semantics. Programs must lower to fixed-size data, statically bounded control flow and helper calls that the kernel verifier can reason about.
