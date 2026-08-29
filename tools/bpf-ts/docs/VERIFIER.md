# Verifier strategy

bpf-ts performs conservative source-level checks before clang, but the kernel verifier remains authoritative. The compiler should gradually learn verifier-log source mapping rather than trying to duplicate the verifier in TypeScript.
