# Quick start

```bash
cd tools/bpf-ts
bun install
bun test
bun src/cli.ts examples/exec.ts --out generated/exec.bpf.c --manifest generated/exec.json
clang -O2 -g -target bpf -D__TARGET_ARCH_x86 -I/usr/include/$(uname -m)-linux-gnu -c generated/exec.bpf.c -o generated/exec.bpf.o
```
