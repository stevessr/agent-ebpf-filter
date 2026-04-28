export type LinuxReferenceKind = "syscall" | "helper";

export const linuxReferenceRelease = "Linux 6.18 LTS" as const;

export interface LinuxReferenceEntry {
  id: string;
  kind: LinuxReferenceKind;
  name: string;
  category: string;
  summary: string;
  synopsis: string;
  url: string;
  localPath: string;
  release: typeof linuxReferenceRelease;
  source: string;
  aliases?: string[];
  keywords?: string[];
}

interface ReferenceSeed {
  name: string;
  category: string;
  summary: string;
  synopsis?: string;
  aliases?: string[];
  keywords?: string[];
}

const man7 = (name: string) =>
  `https://man7.org/linux/man-pages/man2/${name}.2.html`;

const helperDoc = (name: string) =>
  `https://docs.ebpf.io/linux/helper-function/${name}/`;

const localSnapshotPath = (kind: LinuxReferenceKind, name: string) =>
  `/linux-docs/6.18/${kind}/${name}.md`;

const makeEntry = (
  kind: LinuxReferenceKind,
  seed: ReferenceSeed,
): LinuxReferenceEntry => ({
  id: `${kind}-${seed.name}`,
  kind,
  name: seed.name,
  category: seed.category,
  summary: seed.summary,
  synopsis:
    seed.synopsis || (kind === "syscall" ? `${seed.name}(...)` : `${seed.name}(...)`),
  url: kind === "syscall" ? man7(seed.name) : helperDoc(seed.name),
  localPath: localSnapshotPath(kind, seed.name),
  release: linuxReferenceRelease,
  source: kind === "syscall" ? "man7.org" : "docs.ebpf.io",
  aliases: seed.aliases,
  keywords: seed.keywords,
});

const syscallSeeds: ReferenceSeed[] = [
  { name: "execve", category: "Process", summary: "Execute a new program.", synopsis: "execve(path, argv, envp)" },
  { name: "execveat", category: "Process", summary: "Execute a program relative to a directory fd.", synopsis: "execveat(dirfd, path, argv, envp, flags)" },
  { name: "clone", category: "Process", summary: "Create a new process or thread.", synopsis: "clone(flags, stack, parent_tid, child_tid, tls)" },
  { name: "exit_group", category: "Process", summary: "Terminate all threads in the current process.", synopsis: "exit_group(status)" },
  { name: "kill", category: "Process", summary: "Send a signal to a process.", synopsis: "kill(pid, sig)" },
  { name: "tgkill", category: "Process", summary: "Send a signal to a specific thread.", synopsis: "tgkill(tgid, tid, sig)" },
  { name: "tkill", category: "Process", summary: "Send a signal to a thread by TID.", synopsis: "tkill(tid, sig)" },
  { name: "process_vm_readv", category: "Process", summary: "Read memory from another process.", synopsis: "process_vm_readv(pid, local_iov, liovcnt, remote_iov, riovcnt, flags)" },
  { name: "process_vm_writev", category: "Process", summary: "Write memory into another process.", synopsis: "process_vm_writev(pid, local_iov, liovcnt, remote_iov, riovcnt, flags)" },

  { name: "socket", category: "Network", summary: "Create a socket endpoint.", synopsis: "socket(domain, type, protocol)" },
  { name: "connect", category: "Network", summary: "Initiate a connection on a socket.", synopsis: "connect(fd, addr, addrlen)" },
  { name: "bind", category: "Network", summary: "Bind a socket to an address.", synopsis: "bind(fd, addr, addrlen)" },
  { name: "listen", category: "Network", summary: "Mark a socket as listening.", synopsis: "listen(fd, backlog)" },
  { name: "accept", category: "Network", summary: "Accept an incoming connection.", synopsis: "accept(fd, addr, addrlen)" },
  { name: "accept4", category: "Network", summary: "Accept a connection with flags.", synopsis: "accept4(fd, addr, addrlen, flags)" },
  { name: "sendto", category: "Network", summary: "Send a datagram or stream payload.", synopsis: "sendto(fd, buf, len, flags, dest, destlen)" },
  { name: "recvfrom", category: "Network", summary: "Receive data from a socket.", synopsis: "recvfrom(fd, buf, len, flags, src, srclen)" },
  { name: "sendmsg", category: "Network", summary: "Send a message using an msghdr.", synopsis: "sendmsg(fd, msg, flags)" },
  { name: "recvmsg", category: "Network", summary: "Receive a message using an msghdr.", synopsis: "recvmsg(fd, msg, flags)" },
  { name: "shutdown", category: "Network", summary: "Shut down part or all of a socket.", synopsis: "shutdown(fd, how)" },

  { name: "open", category: "Filesystem", summary: "Open a file path.", synopsis: "open(path, flags, mode)" },
  { name: "openat", category: "Filesystem", summary: "Open a path relative to a directory fd.", synopsis: "openat(dirfd, path, flags, mode)" },
  { name: "openat2", category: "Filesystem", summary: "Open a path with extended resolution rules.", synopsis: "openat2(dirfd, path, how, size)" },
  { name: "read", category: "Filesystem", summary: "Read data from a file descriptor.", synopsis: "read(fd, buf, count)" },
  { name: "write", category: "Filesystem", summary: "Write data to a file descriptor.", synopsis: "write(fd, buf, count)" },
  { name: "access", category: "Filesystem", summary: "Check pathname accessibility.", synopsis: "access(path, mode)" },
  { name: "chdir", category: "Filesystem", summary: "Change the working directory.", synopsis: "chdir(path)" },
  { name: "chroot", category: "Filesystem", summary: "Change the process root directory.", synopsis: "chroot(path)" },
  { name: "truncate", category: "Filesystem", summary: "Resize a file by pathname.", synopsis: "truncate(path, length)" },
  { name: "memfd_create", category: "Filesystem", summary: "Create an anonymous in-memory file.", synopsis: "memfd_create(name, flags)" },

  { name: "mkdir", category: "Filesystem", summary: "Create a directory.", synopsis: "mkdir(path, mode)" },
  { name: "mkdirat", category: "Filesystem", summary: "Create a directory relative to a directory fd.", synopsis: "mkdirat(dirfd, path, mode)" },
  { name: "unlink", category: "Filesystem", summary: "Remove a file path.", synopsis: "unlink(path)" },
  { name: "unlinkat", category: "Filesystem", summary: "Remove a file or directory relative to a directory fd.", synopsis: "unlinkat(dirfd, path, flags)" },
  { name: "rename", category: "Filesystem", summary: "Rename or move a file path.", synopsis: "rename(oldpath, newpath)" },
  { name: "renameat2", category: "Filesystem", summary: "Rename or exchange path names with flags.", synopsis: "renameat2(olddirfd, oldpath, newdirfd, newpath, flags)" },
  { name: "linkat", category: "Filesystem", summary: "Create a hard link relative to directory fds.", synopsis: "linkat(olddirfd, oldpath, newdirfd, newpath, flags)" },
  { name: "symlinkat", category: "Filesystem", summary: "Create a symbolic link relative to a directory fd.", synopsis: "symlinkat(target, newdirfd, linkpath)" },
  { name: "chmod", category: "Filesystem", summary: "Change file mode bits.", synopsis: "chmod(path, mode)" },
  { name: "chown", category: "Filesystem", summary: "Change file owner and group.", synopsis: "chown(path, owner, group)" },
  { name: "readlinkat", category: "Filesystem", summary: "Read a symlink target relative to a directory fd.", synopsis: "readlinkat(dirfd, path, buf, bufsiz)" },
  { name: "utimensat", category: "Filesystem", summary: "Update file timestamps.", synopsis: "utimensat(dirfd, path, times, flags)" },

  { name: "mount", category: "Filesystem", summary: "Mount a filesystem.", synopsis: "mount(source, target, filesystemtype, flags, data)" },
  { name: "umount2", category: "Filesystem", summary: "Unmount a filesystem.", synopsis: "umount2(target, flags)" },
  { name: "pivot_root", category: "Filesystem", summary: "Change the root filesystem.", synopsis: "pivot_root(new_root, put_old)" },
  { name: "fsopen", category: "Filesystem", summary: "Open a filesystem context.", synopsis: "fsopen(fs_name, flags)" },
  { name: "open_tree", category: "Filesystem", summary: "Open a mount tree.", synopsis: "open_tree(dfd, filename, flags)" },
  { name: "move_mount", category: "Filesystem", summary: "Move a mount point.", synopsis: "move_mount(from_dfd, from_path, to_dfd, to_path, flags)" },

  { name: "fanotify_mark", category: "Filesystem", summary: "Add or remove a fanotify mark.", synopsis: "fanotify_mark(fd, flags, mask, dirfd, pathname)" },
  { name: "inotify_add_watch", category: "Filesystem", summary: "Add an inotify watch for a path.", synopsis: "inotify_add_watch(fd, pathname, mask)" },

  { name: "ioctl", category: "Device", summary: "Perform a device-specific control operation.", synopsis: "ioctl(fd, request, argp)" },
  { name: "bpf", category: "Security", summary: "Load programs, create maps, and manage BPF objects.", synopsis: "bpf(cmd, attr, size)" },
  { name: "ptrace", category: "Security", summary: "Inspect or control another process.", synopsis: "ptrace(request, pid, addr, data)" },
  { name: "prctl", category: "Security", summary: "Adjust process controls and capabilities.", synopsis: "prctl(option, arg2, arg3, arg4, arg5)" },
  { name: "seccomp", category: "Security", summary: "Install or query seccomp sandbox filters.", synopsis: "seccomp(op, flags, args)" },
  { name: "setns", category: "Security", summary: "Join a namespace from a file descriptor.", synopsis: "setns(fd, nstype)" },
  { name: "unshare", category: "Security", summary: "Unshare namespace or other resources.", synopsis: "unshare(flags)" },

  { name: "request_key", category: "Security", summary: "Request a kernel key from the keyring service.", synopsis: "request_key(type, description, callout_info, dest_keyring)" },
  { name: "keyctl", category: "Security", summary: "Manage kernel keyrings and keys.", synopsis: "keyctl(cmd, ...)" },

  { name: "sethostname", category: "System", summary: "Set the system hostname.", synopsis: "sethostname(name, len)" },
  { name: "setdomainname", category: "System", summary: "Set the system NIS domain name.", synopsis: "setdomainname(name, len)" },
];

const helperSeeds: ReferenceSeed[] = [
  { name: "bpf_map_lookup_elem", category: "Maps", summary: "Look up a value by key in a BPF map.", synopsis: "void *bpf_map_lookup_elem(void *map, const void *key)" },
  { name: "bpf_map_update_elem", category: "Maps", summary: "Insert or update a value in a BPF map.", synopsis: "long bpf_map_update_elem(void *map, const void *key, const void *value, __u64 flags)" },
  { name: "bpf_map_delete_elem", category: "Maps", summary: "Remove a value from a BPF map.", synopsis: "long bpf_map_delete_elem(void *map, const void *key)" },
  { name: "bpf_map_get_next_key", category: "Maps", summary: "Iterate through keys in a BPF map.", synopsis: "long bpf_map_get_next_key(void *map, const void *key, void *next_key)" },

  { name: "bpf_get_current_comm", category: "Task", summary: "Read the current task command name.", synopsis: "long bpf_get_current_comm(char *buf, int size)" },
  { name: "bpf_get_current_pid_tgid", category: "Task", summary: "Return the current PID/TGID pair.", synopsis: "__u64 bpf_get_current_pid_tgid(void)" },
  { name: "bpf_get_current_uid_gid", category: "Task", summary: "Return the current UID/GID pair.", synopsis: "__u64 bpf_get_current_uid_gid(void)" },

  { name: "bpf_probe_read_user_str", category: "Memory", summary: "Read a NUL-terminated string from user memory.", synopsis: "long bpf_probe_read_user_str(void *dst, int size, const void *unsafe_ptr)" },
  { name: "bpf_probe_read_kernel_str", category: "Memory", summary: "Read a NUL-terminated string from kernel memory.", synopsis: "long bpf_probe_read_kernel_str(void *dst, int size, const void *unsafe_ptr)" },

  { name: "bpf_ringbuf_reserve", category: "Ring Buffer", summary: "Reserve space in a ring-buffer map.", synopsis: "void *bpf_ringbuf_reserve(void *ringbuf, __u64 size, __u64 flags)" },
  { name: "bpf_ringbuf_submit", category: "Ring Buffer", summary: "Submit reserved ring-buffer data.", synopsis: "void bpf_ringbuf_submit(void *data, __u64 flags)" },
  { name: "bpf_ringbuf_output", category: "Ring Buffer", summary: "Copy data into a ring-buffer map.", synopsis: "long bpf_ringbuf_output(void *ringbuf, const void *data, __u64 size, __u64 flags)" },

  { name: "bpf_ktime_get_ns", category: "Time", summary: "Read the current monotonic time in nanoseconds.", synopsis: "__u64 bpf_ktime_get_ns(void)" },
  { name: "bpf_get_prandom_u32", category: "Random", summary: "Read a pseudo-random u32 value.", synopsis: "__u32 bpf_get_prandom_u32(void)" },
  { name: "bpf_get_smp_processor_id", category: "CPU", summary: "Return the current CPU id.", synopsis: "__u32 bpf_get_smp_processor_id(void)" },
  { name: "bpf_get_stackid", category: "Stack Trace", summary: "Capture a stack trace id in a map.", synopsis: "long bpf_get_stackid(void *ctx, void *map, __u64 flags)" },
  { name: "bpf_trace_printk", category: "Debug", summary: "Print a trace message to the kernel trace pipe.", synopsis: "long bpf_trace_printk(const char *fmt, int fmt_size, ...)" },
];

export const linuxReferenceCatalog: LinuxReferenceEntry[] = [
  ...syscallSeeds.map((seed) => makeEntry("syscall", seed)),
  ...helperSeeds.map((seed) => makeEntry("helper", seed)),
];

export const linuxReferenceQuickQueries = [
  "openat",
  "execve",
  "bpf",
  "bpf_map_lookup_elem",
  "bpf_probe_read_user_str",
  "seccomp",
  "setns",
  "fanotify_mark",
] as const;

export const linuxReferenceScopes: Array<{
  label: string;
  value: "all" | LinuxReferenceKind;
}> = [
  { label: "All", value: "all" },
  { label: "Syscalls", value: "syscall" },
  { label: "eBPF helpers", value: "helper" },
];
