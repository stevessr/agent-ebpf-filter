# Contributing to bpf-ts

Add syntax only by extending the parser/IR and a backend lowering together. Every new construct needs at least one positive compilation test and one relevant rejection/boundary test. Avoid adding JavaScript runtime semantics that cannot map predictably to eBPF.
