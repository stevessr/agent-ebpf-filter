# bpf-ts verifier-aware subset

The initial compiler intentionally supports a small, statically bounded subset of TypeScript.

Allowed constructs include fixed-layout interfaces, scalar and fixed byte-array fields, map declarations, probe functions, local scalar variables, conditionals, compile-time bounded `for` loops, arithmetic/bitwise comparisons, selected BPF helpers, ring-buffer emission and hash/array updates.

Rejected constructs include `any`, dynamic object shapes, optional struct fields, inheritance, async/generators, recursion, while/do loops, exceptions, closures, dynamic property access and loops whose iteration count cannot be proven at compile time or exceeds the policy limit.

The compiler currently lowers to restricted libbpf C. Native BPF instruction generation is a later backend and must preserve the same IR-level semantic checks.
