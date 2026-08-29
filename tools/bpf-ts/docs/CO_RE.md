# CO-RE direction

CO-RE support is intentionally deferred until the AST/IR boundary is stable. The intended surface is typed kernel-context access that lowers to `BPF_CORE_READ` or equivalent generated accessors, never unrestricted pointer arithmetic from TypeScript.
