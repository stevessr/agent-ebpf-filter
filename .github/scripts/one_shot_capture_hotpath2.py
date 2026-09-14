from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing anchor in {path}: {old[:140]!r}")
    write(path, text.replace(old, new, 1))


index = "backend/app/captureprofile/profile_index.go"
text = read(index)
text = text.replace(
    "matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared, detailed)",
    "matchCompiledProfile(&snapshot.index.compiled[profileIndex], &prepared, detailed)",
)
text = text.replace(
    "matchCompiledProfile(snapshot.index.compiled[profileIndex], prepared, true)",
    "matchCompiledProfile(&snapshot.index.compiled[profileIndex], &prepared, true)",
)
text = text.replace(
    "func matchCompiledProfile(compiled compiledProfile, observation preparedObservation, detailed bool) (Match, bool) {\n\tprofile := compiled.profile",
    "func matchCompiledProfile(compiled *compiledProfile, observation *preparedObservation, detailed bool) (Match, bool) {\n\tprofile := &compiled.profile",
)
text = text.replace(
    "if hostSuffixMatch(observation.Host, suffix) {",
    "if hostSuffixMatchNormalized(observation.Host, suffix) {",
)
write(index, text)

profile = "backend/app/captureprofile/profile.go"
replace_once(profile,
'''func hostSuffixMatch(host, suffix string) bool {
\thost = NormalizeHost(host)
\tsuffix = NormalizeHost(suffix)
\tif host == "" || suffix == "" {
\t\treturn false
\t}
\tif strings.HasPrefix(suffix, ".") {
\t\treturn strings.HasSuffix(host, suffix)
\t}
\treturn host == suffix || strings.HasSuffix(host, "."+suffix)
}''',
'''func hostSuffixMatch(host, suffix string) bool {
\treturn hostSuffixMatchNormalized(NormalizeHost(host), NormalizeHost(suffix))
}

// hostSuffixMatchNormalized is the capture hot-path variant. Registry profiles
// and prepared observations are normalized before matching, so re-running URL,
// case and whitespace normalization for every candidate only burns CPU.
func hostSuffixMatchNormalized(host, suffix string) bool {
\tif host == "" || suffix == "" {
\t\treturn false
\t}
\tif strings.HasPrefix(suffix, ".") {
\t\treturn strings.HasSuffix(host, suffix)
\t}
\treturn host == suffix || strings.HasSuffix(host, "."+suffix)
}''')

common = "backend/ebpf/agent_tracker_common.h"
replace_once(common,
'''#define AF_INET 2
#define AF_INET6 10
''',
'''#define AF_INET 2
#define AF_INET6 10
#define SOCK_STREAM 1
#define SOCK_TYPE_MASK 0xf
''')
replace_once(common,
'''struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 16384);
    __type(key, struct socket_fd_key);
    __type(value, struct socket_fd_meta);
} socket_fds SEC(".maps");
''',
'''struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 16384);
    __type(key, struct socket_fd_key);
    __type(value, struct socket_fd_meta);
} socket_fds SEC(".maps");

// HTTP/1 start-line capture only makes sense on byte-stream sockets. socket(2)
// can OR SOCK_NONBLOCK/SOCK_CLOEXEC into the type argument, so mask to the
// low socket-kind bits instead of comparing the raw value.
static __always_inline int socket_http1_capture_eligible(const struct socket_fd_meta *socket) {
    return socket && ((socket->sock_type & SOCK_TYPE_MASK) == SOCK_STREAM);
}
''')

syscalls = "backend/ebpf/agent_tracker_syscalls.h"
text = read(syscalls)
# Known datagram/raw sockets bypass the user-memory HTTP probe. Unknown socket
# provenance keeps the previous conservative behavior where applicable.
text = text.replace(
    "u32 captured = capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], requested, &start_kind);",
    "u32 captured = socket_http1_capture_eligible(socket) ? capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], requested, &start_kind) : 0;",
    1,
)
text = text.replace(
    "u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);",
    "u32 captured = socket_http1_capture_eligible(socket) ? capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind) : 0;",
    1,
)
# sendmsg has its own socket && iov guard.
text = text.replace(
    "if (socket && have_iov) {\n            u32 start_kind = HTTP1_START_NONE;\n            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);",
    "if (socket && have_iov && socket_http1_capture_eligible(socket)) {\n            u32 start_kind = HTTP1_START_NONE;\n            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);",
    1,
)
# sendto may have explicit sockaddr metadata even when socket provenance is
# missing. Preserve the old fallback for unknown sockets, but skip known
# datagram/raw descriptors.
text = text.replace(
    "u32 captured = capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], data_len, &start_kind);",
    "u32 captured = (!socket || socket_http1_capture_eligible(socket)) ? capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], data_len, &start_kind) : 0;",
    1,
)
# Incoming capture happens after the syscall wrote into userspace. socket_type
# was carried in exit_meta by fill_network_meta_from_socket_direction().
text = text.replace(
    "if (ctx->ret > 0 && meta.extra2 == 1 && pd && meta.addr_ptr != 0) {",
    "if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && pd && meta.addr_ptr != 0) {",
    1,
)
text = text.replace(
    "if (ctx->ret > 0 && meta.extra2 == 1 && pd && meta.addr_ptr != 0 && meta.capture_reserved > 0) {",
    "if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && pd && meta.addr_ptr != 0 && meta.capture_reserved > 0) {",
    1,
)
# recvmsg uses the same post-syscall capture shape later in the file.
text = text.replace(
    "if (ctx->ret > 0 && meta.extra2 == 1 && pd && meta.addr_ptr != 0 && meta.capture_reserved > 0) {",
    "if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && pd && meta.addr_ptr != 0 && meta.capture_reserved > 0) {",
    1,
)
write(syscalls, text)

doc = "docs/backend/generic-api-capture.md"
text = read(doc)
needle = "## Performance"
addition = '''## Hot-path rejection\n\nKernel HTTP/1 metadata probing is restricted to known `SOCK_STREAM` descriptors.\nThe socket type is masked with `SOCK_TYPE_MASK`, so `SOCK_NONBLOCK`/`SOCK_CLOEXEC`\nflags do not accidentally disable capture. Unknown descriptor provenance keeps the\nconservative fallback where the syscall itself still provides a socket endpoint.\nThis avoids user-memory L7 probes on known UDP/raw sockets without changing the\nuserspace profile matcher or privacy boundary.\n\n'''
if addition not in text:
    if needle in text:
        text = text.replace(needle, addition + needle, 1)
    else:
        text += "\n" + addition
write(doc, text)
