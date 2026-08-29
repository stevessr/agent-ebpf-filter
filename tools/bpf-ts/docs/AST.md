# AST boundary

The TypeScript compiler API is used only for parsing familiar syntax. bpf-ts immediately lowers supported nodes into its own small IR; later backends therefore do not depend on TypeScript AST details.
