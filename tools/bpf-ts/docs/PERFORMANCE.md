# Compiler performance

The compiler is a build-time tool. The important runtime performance property is that generated BPF uses ordinary maps/helpers with no TypeScript runtime. Future compiler benchmarks should focus on large generated probe sets and source-to-verifier feedback latency.
