# Security posture

bpf-ts is fail-closed by design. Unsupported source constructs are compile errors. The initial compiler does not attempt to emulate JavaScript semantics and does not silently fall back to generated helper code for unknown calls. Generated programs still require normal kernel verifier validation before loading.
