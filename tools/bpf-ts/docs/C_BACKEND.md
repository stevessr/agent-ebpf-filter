# C backend

The first backend emits intentionally boring libbpf C: fixed structs, standard map-definition macros, normal `SEC()` annotations and explicit helper calls. Readability is preferred over clever lowering so clang/verifier diagnostics remain actionable.
