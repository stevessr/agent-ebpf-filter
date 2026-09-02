# Outputs

The PoC produces readable `.bpf.c` plus an optional attach-manifest JSON. ELF generation remains an explicit clang step so build systems can choose architecture flags, BTF inputs and output paths.
