#![no_std]

pub const MAX_PATH_LEN: usize = 256;
pub const TASK_COMM_LEN: usize = 16;

pub mod event_type {
    pub const EXECVE: u32 = 0;
    pub const OPENAT: u32 = 1;
    pub const CONNECT: u32 = 2;
    pub const MKDIRAT: u32 = 3;
    pub const UNLINKAT: u32 = 4;
    pub const IOCTL: u32 = 5;
    pub const BIND: u32 = 6;
    pub const SENDTO: u32 = 7;
    pub const RECVFROM: u32 = 8;
    pub const READ: u32 = 9;
    pub const WRITE: u32 = 10;
    pub const OPEN: u32 = 11;
    pub const CHMOD: u32 = 12;
    pub const CHOWN: u32 = 13;
    pub const RENAME: u32 = 14;
    pub const LINK: u32 = 15;
    pub const SYMLINK: u32 = 16;
    pub const MKNOD: u32 = 17;
    pub const CLONE: u32 = 18;
    pub const EXIT: u32 = 19;
    pub const SOCKET: u32 = 20;
    pub const ACCEPT: u32 = 21;
    pub const ACCEPT4: u32 = 22;
    pub const GENERIC_SYSCALL: u32 = 25;
    pub const PROCESS_FORK: u32 = 26;
    pub const PROCESS_EXEC: u32 = 27;
    pub const PROCESS_EXIT: u32 = 28;
    pub const WAIT4: u32 = 29;
    pub const SEMANTIC_ALERT: u32 = 30;
    pub const TCP_CONNECT: u32 = 31;
    pub const TCP_CLOSE: u32 = 32;
    pub const TCP_STATE_CHANGE: u32 = 33;
    pub const DNS_QUERY: u32 = 34;
    pub const SOCKET_HTTP: u32 = 43;
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct Event {
    pub pid: u32,
    pub tgid: u32,
    pub ppid: u32,
    pub uid: u32,
    pub gid: u32,
    pub event_type: u32,
    pub tag_id: u32,
    pub comm: [u8; TASK_COMM_LEN],
    pub path: [u8; MAX_PATH_LEN],
    pub net_family: u32,
    pub net_direction: u32,
    pub net_bytes: u32,
    pub net_port: u32,
    pub net_addr: [u8; 16],
    pub retval: i64,
    pub duration_ns: u64,
    pub cgroup_id: u64,
    pub extra1: u32,
    pub extra2: u32,
    pub extra3: u64,
    pub extra4: [u8; MAX_PATH_LEN],
    pub kernel_timestamp_ns: u64,
    pub kernel_sequence: u64,
    pub kernel_cpu: u32,
    pub audit_flags: u32,
    pub kernel_audit_generation: u64,
    pub kernel_dropped_since_last: u64,
    pub kernel_reserve_failures_total: u64,
    pub kernel_capture_flags: u32,
    pub kernel_capture_reserved: u32,
}

impl Event {
    pub const ZERO: Self = Self {
        pid: 0,
        tgid: 0,
        ppid: 0,
        uid: 0,
        gid: 0,
        event_type: 0,
        tag_id: 0,
        comm: [0; TASK_COMM_LEN],
        path: [0; MAX_PATH_LEN],
        net_family: 0,
        net_direction: 0,
        net_bytes: 0,
        net_port: 0,
        net_addr: [0; 16],
        retval: 0,
        duration_ns: 0,
        cgroup_id: 0,
        extra1: 0,
        extra2: 0,
        extra3: 0,
        extra4: [0; MAX_PATH_LEN],
        kernel_timestamp_ns: 0,
        kernel_sequence: 0,
        kernel_cpu: 0,
        audit_flags: 0,
        kernel_audit_generation: 0,
        kernel_dropped_since_last: 0,
        kernel_reserve_failures_total: 0,
        kernel_capture_flags: 0,
        kernel_capture_reserved: 0,
    };
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct CollectorStats {
    pub ringbuf_events_total: u64,
    pub ringbuf_reserve_failed_total: u64,
    pub event_sequence: u64,
    pub pending_dropped_events: u64,
    pub audit_generation: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct ContextPressureStats {
    pub exit_full_update_failures: u64,
    pub exit_compact_update_failures: u64,
    pub single_path_update_failures: u64,
    pub pair_path_update_failures: u64,
    pub socket_fd_update_failures: u64,
    pub socket_parent_update_failures: u64,
    pub exit_io_update_failures: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct ExitMeta {
    pub event_type: u32,
    pub tag_id: u32,
    pub extra1: u32,
    pub extra2: u32,
    pub extra3: u64,
    pub net_family: u32,
    pub net_direction: u32,
    pub net_bytes: u32,
    pub net_port: u32,
    pub net_addr: [u8; 16],
    pub addr_ptr: u64,
    pub start_ns: u64,
    pub capture_flags: u32,
    pub capture_reserved: u32,
    pub socket_type: u32,
    pub socket_reserved: u32,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct ExitIoMeta {
    pub event_type: u32,
    pub tag_id: u32,
    pub extra1: u32,
    pub extra2: u32,
    pub extra3: u64,
    pub addr_ptr: u64,
    pub net_family: u32,
    pub net_port: u32,
    pub net_addr: [u8; 16],
    pub capture_flags: u32,
    pub socket_type: u32,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct ExitCompactMeta {
    pub event_type: u32,
    pub tag_id: u32,
    pub extra1: u32,
    pub extra2: u32,
    pub extra3: u64,
    pub start_ns: u64,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct SocketFdKey {
    pub tgid: u32,
    pub fd: i32,
}

#[repr(C)]
#[derive(Clone, Copy, Default)]
pub struct SocketFdMeta {
    pub family: u32,
    pub sock_type: u32,
    pub protocol: u32,
    pub remote_port: u32,
    pub remote_addr: [u8; 16],
    pub provenance_flags: u32,
    pub reserved: u32,
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct ExitSinglePathData {
    pub path: [u8; MAX_PATH_LEN],
}

impl ExitSinglePathData {
    pub const ZERO: Self = Self {
        path: [0; MAX_PATH_LEN],
    };
}

#[repr(C)]
#[derive(Clone, Copy)]
pub struct ExitPathData {
    pub path: [u8; MAX_PATH_LEN],
    pub extra4: [u8; MAX_PATH_LEN],
}

impl ExitPathData {
    pub const ZERO: Self = Self {
        path: [0; MAX_PATH_LEN],
        extra4: [0; MAX_PATH_LEN],
    };
}

const _: [(); 688] = [(); core::mem::size_of::<Event>()];
const _: [(); 40] = [(); core::mem::size_of::<CollectorStats>()];
const _: [(); 56] = [(); core::mem::size_of::<ContextPressureStats>()];
const _: [(); 88] = [(); core::mem::size_of::<ExitMeta>()];
const _: [(); 64] = [(); core::mem::size_of::<ExitIoMeta>()];
const _: [(); 32] = [(); core::mem::size_of::<ExitCompactMeta>()];
const _: [(); 8] = [(); core::mem::size_of::<SocketFdKey>()];
const _: [(); 40] = [(); core::mem::size_of::<SocketFdMeta>()];
const _: [(); 256] = [(); core::mem::size_of::<ExitSinglePathData>()];
const _: [(); 512] = [(); core::mem::size_of::<ExitPathData>()];

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn c_abi_sizes_remain_frozen() {
        assert_eq!(core::mem::size_of::<Event>(), 688);
        assert_eq!(core::mem::size_of::<CollectorStats>(), 40);
        assert_eq!(core::mem::size_of::<ContextPressureStats>(), 56);
        assert_eq!(core::mem::size_of::<ExitMeta>(), 88);
        assert_eq!(core::mem::size_of::<ExitIoMeta>(), 64);
        assert_eq!(core::mem::size_of::<ExitCompactMeta>(), 32);
        assert_eq!(core::mem::size_of::<SocketFdKey>(), 8);
        assert_eq!(core::mem::size_of::<SocketFdMeta>(), 40);
        assert_eq!(core::mem::size_of::<ExitSinglePathData>(), 256);
        assert_eq!(core::mem::size_of::<ExitPathData>(), 512);
    }
}
