# Review checklist

- Unsupported syntax fails closed.
- Loop bounds are compile-time and <= policy limit.
- Generated C contains no hidden userspace runtime dependency.
- New helpers have explicit lowering and tests.
- Attach metadata is preserved in the manifest.
- Generated examples compile with clang's BPF target.
