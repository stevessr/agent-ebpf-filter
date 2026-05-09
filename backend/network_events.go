package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"agent-ebpf-filter/pb"
)

func kernelEventTypeName(eventType uint32) string {
	switch eventType {
	case 0:
		return "execve"
	case 1:
		return "openat"
	case 2:
		return "network_connect"
	case 3:
		return "mkdir"
	case 4:
		return "unlink"
	case 5:
		return "ioctl"
	case 6:
		return "network_bind"
	case 7:
		return "network_sendto"
	case 8:
		return "network_recvfrom"
	case 9:
		return "read"
	case 10:
		return "write"
	case 11:
		return "open"
	case 12:
		return "chmod"
	case 13:
		return "chown"
	case 14:
		return "rename"
	case 15:
		return "link"
	case 16:
		return "symlink"
	case 17:
		return "mknod"
	case 18:
		return "clone"
	case 19:
		return "exit"
	case 20:
		return "socket"
	case 21:
		return "accept"
	case 22:
		return "accept4"
	case 25:
		return "syscall"
	case 26:
		return "process_fork"
	case 27:
		return "process_exec"
	case 28:
		return "process_exit"
	case 29:
		return "wait4"
	case 30:
		return "semantic_alert"
	case 31:
		return "tcp_connect"
	case 32:
		return "tcp_close"
	case 33:
		return "tcp_state_change"
	case 34:
		return "dns_query"
	default:
		return "unknown"
	}
}

func isNetworkEventType(eventType string) bool {
	switch eventType {
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom",
		"accept", "accept4", "socket",
		"tcp_connect", "tcp_close", "tcp_state_change", "dns_query":
		return true
	default:
		return false
	}
}

func networkDirectionLabel(direction uint32) string {
	switch direction {
	case 1:
		return "outgoing"
	case 2:
		return "incoming"
	case 3:
		return "listening"
	default:
		return ""
	}
}

func networkFamilyLabel(family uint32) string {
	switch family {
	case 2:
		return "ipv4"
	case 10:
		return "ipv6"
	default:
		return ""
	}
}

func networkIP(family uint32, addr [16]byte) net.IP {
	switch family {
	case 2:
		return net.IP(addr[:4]).To4()
	case 10:
		return net.IP(addr[:]).To16()
	default:
		return nil
	}
}

func formatNetworkEndpoint(family uint32, addr [16]byte, port uint32) string {
	ip := networkIP(family, addr)
	if ip == nil {
		return ""
	}

	host := ip.String()
	if port == 0 {
		return host
	}
	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
}

func formatNetworkSummary(direction, endpoint string, bytes uint32) string {
	if endpoint == "" && bytes == 0 {
		return ""
	}

	parts := make([]string, 0, 3)
	if direction != "" {
		parts = append(parts, direction)
	}
	if endpoint != "" {
		parts = append(parts, endpoint)
	}
	if bytes > 0 {
		parts = append(parts, fmt.Sprintf("(%d B)", bytes))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// sanitizeUTF8 converts a raw byte slice from the kernel to a valid UTF-8 string,
// replacing any invalid bytes with the Unicode replacement character.
func sanitizeUTF8(b []byte) string {
	return strings.ToValidUTF8(strings.TrimRight(string(b), "\x00"), "�")
}

func syscallName(nr uint32) string {
	switch nr {
	case 0:
		return "read"
	case 1:
		return "write"
	case 2:
		return "open"
	case 3:
		return "close"
	case 4:
		return "stat"
	case 5:
		return "fstat"
	case 6:
		return "lstat"
	case 7:
		return "poll"
	case 8:
		return "lseek"
	case 9:
		return "mmap"
	case 10:
		return "mprotect"
	case 11:
		return "munmap"
	case 12:
		return "brk"
	case 13:
		return "rt_sigaction"
	case 14:
		return "rt_sigprocmask"
	case 16:
		return "ioctl"
	case 17:
		return "pread64"
	case 18:
		return "pwrite64"
	case 19:
		return "readv"
	case 20:
		return "writev"
	case 21:
		return "access"
	case 22:
		return "pipe"
	case 23:
		return "select"
	case 24:
		return "sched_yield"
	case 25:
		return "mremap"
	case 26:
		return "msync"
	case 27:
		return "mincore"
	case 28:
		return "madvise"
	case 29:
		return "shmget"
	case 30:
		return "shmat"
	case 31:
		return "shmctl"
	case 32:
		return "dup"
	case 33:
		return "dup2"
	case 34:
		return "pause"
	case 35:
		return "nanosleep"
	case 36:
		return "getitimer"
	case 37:
		return "alarm"
	case 38:
		return "setitimer"
	case 39:
		return "getpid"
	case 40:
		return "sendfile"
	case 41:
		return "socket"
	case 42:
		return "connect"
	case 43:
		return "accept"
	case 44:
		return "sendto"
	case 45:
		return "recvfrom"
	case 46:
		return "sendmsg"
	case 47:
		return "recvmsg"
	case 48:
		return "shutdown"
	case 49:
		return "bind"
	case 50:
		return "listen"
	case 51:
		return "getsockname"
	case 52:
		return "getpeername"
	case 53:
		return "socketpair"
	case 54:
		return "setsockopt"
	case 55:
		return "getsockopt"
	case 56:
		return "clone"
	case 57:
		return "fork"
	case 58:
		return "vfork"
	case 59:
		return "execve"
	case 60:
		return "exit"
	case 61:
		return "wait4"
	case 62:
		return "kill"
	case 63:
		return "uname"
	case 64:
		return "semget"
	case 65:
		return "semop"
	case 66:
		return "semctl"
	case 67:
		return "shmdt"
	case 68:
		return "msgget"
	case 69:
		return "msgsnd"
	case 70:
		return "msgrcv"
	case 71:
		return "msgctl"
	case 72:
		return "fcntl"
	case 73:
		return "flock"
	case 74:
		return "fsync"
	case 75:
		return "fdatasync"
	case 76:
		return "truncate"
	case 77:
		return "ftruncate"
	case 78:
		return "getdents"
	case 79:
		return "getcwd"
	case 80:
		return "chdir"
	case 81:
		return "fchdir"
	case 82:
		return "rename"
	case 83:
		return "mkdir"
	case 84:
		return "rmdir"
	case 85:
		return "creat"
	case 86:
		return "link"
	case 87:
		return "unlink"
	case 88:
		return "symlink"
	case 89:
		return "readlink"
	case 90:
		return "chmod"
	case 91:
		return "fchmod"
	case 92:
		return "chown"
	case 93:
		return "fchown"
	case 94:
		return "lchown"
	case 95:
		return "umask"
	case 96:
		return "gettimeofday"
	case 97:
		return "getrlimit"
	case 98:
		return "getrusage"
	case 99:
		return "sysinfo"
	case 100:
		return "times"
	case 101:
		return "ptrace"
	case 102:
		return "getuid"
	case 103:
		return "syslog"
	case 104:
		return "getgid"
	case 105:
		return "setuid"
	case 106:
		return "setgid"
	case 107:
		return "geteuid"
	case 108:
		return "getegid"
	case 109:
		return "setpgid"
	case 110:
		return "getppid"
	case 111:
		return "getpgrp"
	case 112:
		return "setsid"
	case 113:
		return "setreuid"
	case 114:
		return "setregid"
	case 115:
		return "getgroups"
	case 116:
		return "setgroups"
	case 117:
		return "setresuid"
	case 118:
		return "getresuid"
	case 119:
		return "setresgid"
	case 120:
		return "getresgid"
	case 121:
		return "getpgid"
	case 122:
		return "setfsuid"
	case 123:
		return "setfsgid"
	case 124:
		return "getsid"
	case 125:
		return "capget"
	case 126:
		return "capset"
	case 127:
		return "rt_sigpending"
	case 128:
		return "rt_sigtimedwait"
	case 129:
		return "rt_sigqueueinfo"
	case 130:
		return "rt_sigsuspend"
	case 131:
		return "sigaltstack"
	case 132:
		return "utime"
	case 133:
		return "mknod"
	case 135:
		return "personality"
	case 136:
		return "ustat"
	case 137:
		return "statfs"
	case 138:
		return "fstatfs"
	case 139:
		return "sysfs"
	case 140:
		return "getpriority"
	case 141:
		return "setpriority"
	case 142:
		return "sched_setparam"
	case 143:
		return "sched_getparam"
	case 144:
		return "sched_setscheduler"
	case 145:
		return "sched_getscheduler"
	case 146:
		return "sched_get_priority_max"
	case 147:
		return "sched_get_priority_min"
	case 148:
		return "sched_rr_get_interval"
	case 149:
		return "mlock"
	case 150:
		return "munlock"
	case 151:
		return "mlockall"
	case 152:
		return "munlockall"
	case 153:
		return "vhangup"
	case 154:
		return "modify_ldt"
	case 155:
		return "pivot_root"
	case 157:
		return "prctl"
	case 158:
		return "arch_prctl"
	case 159:
		return "adjtimex"
	case 160:
		return "setrlimit"
	case 161:
		return "chroot"
	case 162:
		return "sync"
	case 163:
		return "acct"
	case 164:
		return "settimeofday"
	case 165:
		return "mount"
	case 166:
		return "umount2"
	case 167:
		return "swapon"
	case 168:
		return "swapoff"
	case 169:
		return "reboot"
	case 170:
		return "sethostname"
	case 171:
		return "setdomainname"
	case 172:
		return "iopl"
	case 173:
		return "ioperm"
	case 175:
		return "init_module"
	case 176:
		return "delete_module"
	case 179:
		return "quotactl"
	case 186:
		return "gettid"
	case 187:
		return "readahead"
	case 188:
		return "setxattr"
	case 189:
		return "lsetxattr"
	case 190:
		return "fsetxattr"
	case 191:
		return "getxattr"
	case 192:
		return "lgetxattr"
	case 193:
		return "fgetxattr"
	case 194:
		return "listxattr"
	case 195:
		return "llistxattr"
	case 196:
		return "flistxattr"
	case 197:
		return "removexattr"
	case 198:
		return "lremovexattr"
	case 199:
		return "fremovexattr"
	case 200:
		return "tkill"
	case 201:
		return "time"
	case 202:
		return "futex"
	case 203:
		return "sched_setaffinity"
	case 204:
		return "sched_getaffinity"
	case 206:
		return "io_setup"
	case 207:
		return "io_destroy"
	case 208:
		return "io_getevents"
	case 209:
		return "io_submit"
	case 210:
		return "io_cancel"
	case 212:
		return "lookup_dcookie"
	case 213:
		return "epoll_create"
	case 216:
		return "remap_file_pages"
	case 217:
		return "getdents64"
	case 218:
		return "set_tid_address"
	case 219:
		return "restart_syscall"
	case 220:
		return "semtimedop"
	case 221:
		return "fadvise64"
	case 222:
		return "timer_create"
	case 223:
		return "timer_settime"
	case 224:
		return "timer_gettime"
	case 225:
		return "timer_getoverrun"
	case 226:
		return "timer_delete"
	case 227:
		return "clock_settime"
	case 228:
		return "clock_gettime"
	case 229:
		return "clock_getres"
	case 230:
		return "clock_nanosleep"
	case 231:
		return "exit_group"
	case 232:
		return "epoll_wait"
	case 233:
		return "epoll_ctl"
	case 234:
		return "tgkill"
	case 235:
		return "utimes"
	case 237:
		return "mbind"
	case 238:
		return "set_mempolicy"
	case 239:
		return "get_mempolicy"
	case 240:
		return "mq_open"
	case 241:
		return "mq_unlink"
	case 242:
		return "mq_timedsend"
	case 243:
		return "mq_timedreceive"
	case 244:
		return "mq_notify"
	case 245:
		return "mq_getsetattr"
	case 246:
		return "kexec_load"
	case 247:
		return "waitid"
	case 248:
		return "add_key"
	case 249:
		return "request_key"
	case 250:
		return "keyctl"
	case 251:
		return "ioprio_set"
	case 252:
		return "ioprio_get"
	case 253:
		return "inotify_init"
	case 254:
		return "inotify_add_watch"
	case 255:
		return "inotify_rm_watch"
	case 256:
		return "migrate_pages"
	case 257:
		return "openat"
	case 258:
		return "mkdirat"
	case 259:
		return "mknodat"
	case 260:
		return "fchownat"
	case 261:
		return "futimesat"
	case 262:
		return "newfstatat"
	case 263:
		return "unlinkat"
	case 264:
		return "renameat"
	case 265:
		return "linkat"
	case 266:
		return "symlinkat"
	case 267:
		return "readlinkat"
	case 268:
		return "fchmodat"
	case 269:
		return "faccessat"
	case 270:
		return "pselect6"
	case 271:
		return "ppoll"
	case 272:
		return "unshare"
	case 273:
		return "set_robust_list"
	case 274:
		return "get_robust_list"
	case 275:
		return "splice"
	case 276:
		return "tee"
	case 277:
		return "sync_file_range"
	case 278:
		return "vmsplice"
	case 279:
		return "move_pages"
	case 280:
		return "utimensat"
	case 281:
		return "epoll_pwait"
	case 282:
		return "signalfd"
	case 283:
		return "timerfd_create"
	case 284:
		return "eventfd"
	case 285:
		return "fallocate"
	case 286:
		return "timerfd_settime"
	case 287:
		return "timerfd_gettime"
	case 288:
		return "accept4"
	case 289:
		return "signalfd4"
	case 290:
		return "eventfd2"
	case 291:
		return "epoll_create1"
	case 292:
		return "dup3"
	case 293:
		return "pipe2"
	case 294:
		return "inotify_init1"
	case 295:
		return "preadv"
	case 296:
		return "pwritev"
	case 297:
		return "rt_tgsigqueueinfo"
	case 298:
		return "perf_event_open"
	case 299:
		return "recvmmsg"
	case 300:
		return "fanotify_init"
	case 301:
		return "fanotify_mark"
	case 302:
		return "prlimit64"
	case 303:
		return "name_to_handle_at"
	case 304:
		return "open_by_handle_at"
	case 305:
		return "clock_adjtime"
	case 306:
		return "syncfs"
	case 307:
		return "sendmmsg"
	case 308:
		return "setns"
	case 309:
		return "getcpu"
	case 310:
		return "process_vm_readv"
	case 311:
		return "process_vm_writev"
	case 312:
		return "kcmp"
	case 313:
		return "finit_module"
	case 314:
		return "sched_setattr"
	case 315:
		return "sched_getattr"
	case 316:
		return "renameat2"
	case 317:
		return "seccomp"
	case 318:
		return "getrandom"
	case 319:
		return "memfd_create"
	case 320:
		return "kexec_file_load"
	case 321:
		return "bpf"
	case 322:
		return "execveat"
	case 323:
		return "userfaultfd"
	case 324:
		return "membarrier"
	case 325:
		return "mlock2"
	case 326:
		return "copy_file_range"
	case 327:
		return "preadv2"
	case 328:
		return "pwritev2"
	case 329:
		return "pkey_mprotect"
	case 330:
		return "pkey_alloc"
	case 331:
		return "pkey_free"
	case 332:
		return "statx"
	case 333:
		return "io_pgetevents"
	case 334:
		return "rseq"
	case 424:
		return "pidfd_send_signal"
	case 425:
		return "io_uring_setup"
	case 426:
		return "io_uring_enter"
	case 427:
		return "io_uring_register"
	case 428:
		return "open_tree"
	case 429:
		return "move_mount"
	case 430:
		return "fsopen"
	case 431:
		return "fsconfig"
	case 432:
		return "fsmount"
	case 433:
		return "fspick"
	case 434:
		return "pidfd_open"
	case 435:
		return "clone3"
	case 436:
		return "close_range"
	case 437:
		return "openat2"
	case 438:
		return "pidfd_getfd"
	case 439:
		return "faccessat2"
	case 440:
		return "process_madvise"
	case 441:
		return "epoll_pwait2"
	case 442:
		return "mount_setattr"
	case 443:
		return "quotactl_fd"
	case 444:
		return "landlock_create_ruleset"
	case 445:
		return "landlock_add_rule"
	case 446:
		return "landlock_restrict_self"
	case 447:
		return "memfd_secret"
	case 448:
		return "process_mrelease"
	case 449:
		return "futex_waitv"
	case 450:
		return "set_mempolicy_home_node"
	case 451:
		return "cachestat"
	case 452:
		return "fchmodat2"
	case 453:
		return "map_shadow_stack"
	}
	return ""
}

func buildKernelEvent(event bpfEvent) *pb.Event {
	comm := sanitizeUTF8(event.Comm[:])
	path := sanitizeUTF8(event.Path[:])
	extraPath := sanitizeUTF8(event.Extra4[:])
	typeName := kernelEventTypeName(event.Type)

	out := &pb.Event{
		Pid:           event.PID,
		Ppid:          event.PPID,
		Uid:           event.UID,
		Gid:           event.GID,
		Tgid:          event.TGID,
		Type:          typeName,
		EventType:     pb.EventType(event.Type),
		Tag:           getTagName(event.TagID),
		Comm:          comm,
		Path:          path,
		Retval:        event.Retval,
		DurationNs:    event.DurationNs,
		CgroupId:      event.CgroupID,
		ExtraPath:     extraPath,
		SchemaVersion: eventSchemaVersion,
	}

	// Populate type-specific fields
	switch typeName {
	case "read", "write":
		out.ExtraInfo = fmt.Sprintf("fd=%d count=%d", event.Extra1, event.Extra3)
		out.Bytes = event.Extra3
	case "open":
		out.ExtraInfo = fmt.Sprintf("flags=0x%x mode=0%o", event.Extra1, event.Extra2)
		out.Mode = fmt.Sprintf("0%o", event.Extra2)
	case "chmod":
		out.Mode = fmt.Sprintf("0%o", event.Extra2)
		out.ExtraInfo = fmt.Sprintf("mode=0%o", event.Extra2)
	case "chown":
		out.UidArg = event.Extra1
		out.GidArg = event.Extra2
		out.ExtraInfo = fmt.Sprintf("uid=%d gid=%d", event.Extra1, event.Extra2)
	case "rename":
		out.ExtraInfo = fmt.Sprintf("newpath=%s", extraPath)
	case "link", "symlink":
		out.ExtraInfo = fmt.Sprintf("target=%s", extraPath)
	case "mknod":
		out.Mode = fmt.Sprintf("0%o", event.Extra1)
		out.ExtraInfo = fmt.Sprintf("mode=0%o dev=0x%x", event.Extra1, event.Extra2)
	case "ioctl":
		out.ExtraInfo = fmt.Sprintf("request=0x%x", event.Extra1)
	case "clone":
		out.ExtraInfo = fmt.Sprintf("flags=0x%x", event.Extra1)
		if event.Retval > 0 {
			out.ExtraInfo += fmt.Sprintf(" child_pid=%d", event.Retval)
		}
	case "exit":
		out.ExtraInfo = fmt.Sprintf("status=%d", event.Extra1)
	case "process_fork":
		out.ExtraInfo = fmt.Sprintf("child_pid=%d", event.Extra1)
		if path == "" {
			out.Path = fmt.Sprintf("pid=%d", event.Extra1)
		}
	case "process_exec":
		out.ExtraInfo = fmt.Sprintf("old_pid=%d", event.Extra1)
	case "process_exit":
		out.ExtraInfo = fmt.Sprintf("group_dead=%t", event.Extra1 != 0)
	case "wait4":
		out.ExtraInfo = fmt.Sprintf("target_pid=%d options=0x%x", int32(event.Extra1), event.Extra2)
	case "socket":
		out.Domain = networkFamilyLabel(event.Extra1)
		if event.Extra1 == 1 {
			out.Domain = "unix"
		}
		switch event.Extra2 {
		case 1:
			out.SockType = "SOCK_STREAM"
		case 2:
			out.SockType = "SOCK_DGRAM"
		case 3:
			out.SockType = "SOCK_RAW"
		default:
			out.SockType = fmt.Sprintf("type=%d", event.Extra2)
		}
		out.Protocol = uint32(event.Extra3)
		out.ExtraInfo = fmt.Sprintf("domain=%s type=%s protocol=%d", out.Domain, out.SockType, out.Protocol)
	case "unlinkat":
		out.ExtraInfo = fmt.Sprintf("flags=0x%x", event.Extra1)
	case "mkdirat":
		out.Mode = fmt.Sprintf("0%o", event.Extra1)
		out.ExtraInfo = fmt.Sprintf("mode=0%o", event.Extra1)
	case "syscall":
		name := syscallName(event.Extra1)
		if name != "" {
			out.ExtraInfo = fmt.Sprintf("%s(%d)", name, event.Extra1)
		} else {
			out.ExtraInfo = fmt.Sprintf("nr=%d", event.Extra1)
		}
		if event.Extra2 != 0 {
			out.ExtraInfo += fmt.Sprintf(" arg=%d", event.Extra2)
		}
		if event.Extra3 != 0 {
			out.ExtraInfo += fmt.Sprintf(" arg2=%d", event.Extra3)
		}
		if event.Retval < 0 {
			out.ExtraInfo += fmt.Sprintf(" err=%d", event.Retval)
		}
	case "tcp_connect":
		saddr := formatIPv4Addr(event.Extra2)
		daddr := formatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.NetBytes = event.NetBytes
		out.ExtraInfo = fmt.Sprintf("saddr=%s sport=%d dport=%d", saddr, event.Extra1, event.NetPort)
	case "tcp_close":
		daddr := formatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.ExtraInfo = fmt.Sprintf("sport=%d dport=%d", event.Extra1, event.NetPort)
	case "tcp_state_change":
		oldState := uint8(event.DurationNs >> 32)
		newState := uint8(event.DurationNs & 0xFFFFFFFF)
		daddr := formatIPv4Addr(uint32(event.Extra3))
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("%s:%d", daddr, event.NetPort)
		out.ExtraInfo = fmt.Sprintf("%s->%s sport=%d dport=%d",
			tcpStateName(oldState), tcpStateName(newState), event.Extra1, event.NetPort)
	case "dns_query":
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("dns:%d", event.NetPort)
		out.Domain = sanitizeUTF8(event.Path[:])
	default:
		if event.Retval != 0 {
			out.ExtraInfo = fmt.Sprintf("retval=%d", event.Retval)
		}
	}

	if typeName == "accept" || typeName == "accept4" || isNetworkEventType(typeName) {
		direction := networkDirectionLabel(event.NetDirection)
		endpoint := formatNetworkEndpoint(event.NetFamily, event.NetAddr, event.NetPort)
		family := networkFamilyLabel(event.NetFamily)
		summary := formatNetworkSummary(direction, endpoint, event.NetBytes)
		if summary != "" {
			out.Path = summary
		}
		out.NetDirection = direction
		out.NetEndpoint = endpoint
		out.NetBytes = event.NetBytes
		out.NetFamily = family
	}

	// Record TCP state and flow for network events
	if isNetworkEventType(typeName) {
		srcIP := formatIPv4Addr(event.Extra2)
		dstIP := formatIPv4Addr(uint32(event.Extra3))
		srcPort := event.NetBytes
		dstPort := event.NetPort

		// Generic syscall tracepoints (network_connect, network_sendto,
		// network_recvfrom, etc.) store addresses in NetAddr, not
		// Extra2/Extra3. Extra3 contains byte counts for sendto/recvfrom
		// which would produce bogus flow keys.
		//
		// TCP flow tracepoints (tcp_connect, tcp_close, tcp_state_change)
		// pack addresses into Extra2/Extra3 via emit_tcp_flow_event.
		switch typeName {
		case "network_sendto", "network_recvfrom":
			// Handled by recordUDPFlowFromEvent below — skip the TCP path.
			srcIP, dstIP = "0.0.0.0", "0.0.0.0"
		case "network_connect":
			// Tagged processes get a full tcp_connect event with both
			// srcIP and dstIP from Extra2/Extra3.  Untagged processes
			// only see this generic event — extract dstIP from NetAddr.
			if event.TagID == 0 {
				if addr := networkIP(event.NetFamily, event.NetAddr); addr != nil {
					if s := addr.String(); s != "" && s != "<nil>" {
						dstIP = s
					}
				}
			} else {
				srcIP, dstIP = "0.0.0.0", "0.0.0.0"
			}
		}

		if srcIP != "0.0.0.0" && dstIP != "0.0.0.0" && dstPort > 0 {
			applyBestEffortProcessContextToEvent(out)
			flowState := ""
			switch typeName {
			case "tcp_close":
				flowState = "CLOSED"
			case "tcp_state_change":
				flowState = tcpStateName(uint8(event.DurationNs & 0xFFFFFFFF))
			}
			populateEventFlowFields(out, srcIP, dstIP, srcPort, dstPort, "TCP")
			recordNetworkFlowContextFromEvent(srcIP, dstIP, srcPort, dstPort, out, flowState)
			globalBandwidthTracker.RecordBytes(srcIP, dstIP, dstPort, "TCP", out.NetDirection, uint64(out.NetBytes), out.Comm, out.Pid)
			// Protocol detection from captured payload
			if extraPath := sanitizeUTF8(event.Extra4[:]); len(extraPath) > 4 {
				entry := detectAndRecordProtocol(dstIP, dstPort, []byte(extraPath))
				networkFlowAggregator.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, "TCP", entry)
				applyProtocolMetadataToEvent(out, entry)
				if entry != nil && entry.SNI != "" {
					out.Domain = entry.SNI
					out.NetEndpoint = fmt.Sprintf("%s:%d [SNI: %s]", dstIP, dstPort, entry.SNI)
				}
				if entry != nil && entry.HTTPHost != "" {
					out.Domain = entry.HTTPHost
					out.NetEndpoint = fmt.Sprintf("%s:%d [Host: %s]", dstIP, dstPort, entry.HTTPHost)
				}
			}
		}
		// TCP state tracking
		switch typeName {
		case "tcp_connect":
			tcpTracker.RecordConnect(srcIP, dstIP, srcPort, dstPort, out.Pid, out.Comm)
		case "tcp_close":
			tcpTracker.RecordClose(srcIP, dstIP, srcPort, dstPort)
		case "tcp_state_change":
			oldState := uint8(event.DurationNs >> 32)
			newState := uint8(event.DurationNs & 0xFFFFFFFF)
			tcpTracker.RecordStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, out.Pid, out.Comm)
		}
		if (typeName == "network_sendto" || typeName == "network_recvfrom") && dstPort > 0 {
			recordUDPFlowFromEvent(event, out)
		}
	}

	return out
}

func recordUDPFlowFromEvent(event bpfEvent, out *pb.Event) {
	if out == nil {
		return
	}
	remoteIP := networkIP(event.NetFamily, event.NetAddr)
	if remoteIP == nil {
		return
	}
	remote := remoteIP.String()
	if remote == "" || remote == "<nil>" {
		return
	}
	srcIP, dstIP := "local", remote
	srcPort, dstPort := uint32(0), event.NetPort
	if out.GetNetDirection() == "incoming" {
		srcIP, dstIP = remote, "local"
	}
	recordNetworkFlowContextFromEvent(srcIP, dstIP, srcPort, dstPort, out, "")
	populateEventFlowFields(out, srcIP, dstIP, srcPort, dstPort, "UDP")
	if extraPath := sanitizeUTF8(event.Extra4[:]); len(extraPath) > 4 {
		entry := detectAndRecordProtocol(remote, dstPort, []byte(extraPath))
		networkFlowAggregator.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, "UDP", entry)
		applyProtocolMetadataToEvent(out, entry)
	}
}

func populateEventFlowFields(out *pb.Event, srcIP, dstIP string, srcPort, dstPort uint32, transport string) {
	if out == nil {
		return
	}
	key := makeFlowKey(srcIP, dstIP, srcPort, dstPort, transport)
	out.FlowId = key.ID()
	out.SrcIp = srcIP
	out.SrcPort = srcPort
	out.DstIp = dstIP
	out.DstPort = dstPort
	out.Transport = transport
	out.ServiceName = lookupServiceByPort(dstPort)
	out.IpScope = string(classifyIPScope(netParseIPForFlow(dstIP)))
	if domain, ok := dnsCorrelation.LookupIP(dstIP); ok {
		out.DnsName = domain
		if out.Domain == "" {
			out.Domain = domain
		}
	}
	out.AppProtocol = detectAppProtocol(dstPort, out.Domain)
	if out.NetDirection == "incoming" {
		out.BytesIn = uint64(out.NetBytes)
		out.PacketsIn = 1
	} else if out.NetDirection == "outgoing" {
		out.BytesOut = uint64(out.NetBytes)
		out.PacketsOut = 1
	}
}

func applyProtocolMetadataToEvent(out *pb.Event, entry *protoDetectionEntry) {
	if out == nil || entry == nil {
		return
	}
	out.AppProtocol = string(entry.AppProtocol)
	if entry.SNI != "" {
		out.Sni = entry.SNI
		if out.Domain == "" {
			out.Domain = entry.SNI
		}
	}
	if entry.ALPN != "" {
		out.TlsAlpn = entry.ALPN
	}
	if entry.HTTPHost != "" {
		out.HttpHost = entry.HTTPHost
		if out.Domain == "" || out.Domain == out.Sni {
			out.Domain = entry.HTTPHost
		}
		if entry.AppProtocol == AppProtoDNS || entry.AppProtocol == AppProtomDNS {
			out.DnsName = entry.HTTPHost
		}
	}
}

func netParseIPForFlow(ip string) net.IP {
	if ip == "local" {
		return net.ParseIP("127.0.0.1")
	}
	return net.ParseIP(ip)
}

func formatIPv4Addr(addr uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		addr&0xFF, (addr>>8)&0xFF, (addr>>16)&0xFF, (addr>>24)&0xFF)
}

var tcpStateNames = map[uint8]string{
	1:  "ESTABLISHED",
	2:  "SYN_SENT",
	3:  "SYN_RECV",
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
	11: "CLOSING",
}

func tcpStateName(state uint8) string {
	if name, ok := tcpStateNames[state]; ok {
		return name
	}
	return fmt.Sprintf("STATE_%d", state)
}
