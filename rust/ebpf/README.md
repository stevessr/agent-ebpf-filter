# Aya eBPF rewrite

The Aya program is deliberately kept outside the stable userspace workspace because eBPF uses a separate target/toolchain.

The current program is a bootstrap tracepoint only. It validates the Rust/Aya loading and ring-buffer direction; it is **not** a drop-in replacement for the existing C collector yet.

Migration order:

1. Freeze and reproduce the existing event/map ABIs.
2. Port tracker maps and syscall tracepoints.
3. Port cgroup network enforcement.
4. Port BPF LSM file/exec enforcement.
5. Add verifier/load regression tests.
6. Delete the corresponding C source only after parity tests pass.
