# Initial helpers

Expression helpers: `bpf.pid()`, `bpf.tid()`, `bpf.uid()`, `bpf.gid()`, `bpf.ktimeNs()`, `bpf.arg(ctx, 1..5)`.

Ring-buffer byte fields additionally support `bpf.comm()` and `bpf.userString(ptr)`. Addressable locals can be filled with `bpf.readUser(target, ptr)`.
