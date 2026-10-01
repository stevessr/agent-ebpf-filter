#![no_std]
#![no_main]

use agent_ebpf_types::{
    CollectorStats, ContextPressureStats, Event, ExitCompactMeta, ExitIoMeta, ExitMeta,
    ExitPathData, ExitSinglePathData, SocketFdKey, SocketFdMeta, event_type,
};
use aya_ebpf::{
    helpers::{
        bpf_get_current_comm, bpf_get_current_pid_tgid, bpf_get_current_uid_gid,
        generated::{bpf_get_current_cgroup_id, bpf_get_smp_processor_id, bpf_ktime_get_ns},
    },
    macros::{map, tracepoint},
    maps::{HashMap, LruHashMap, PerCpuArray, RingBuf},
    programs::TracePointContext,
};

#[map(name = "events")]
static EVENTS: RingBuf = RingBuf::with_byte_size(256 * 1024, 0);

#[map(name = "collector_stats")]
static COLLECTOR_STATS: PerCpuArray<CollectorStats> = PerCpuArray::with_max_entries(1, 0);

#[map(name = "context_pressure_stats")]
static CONTEXT_PRESSURE_STATS: PerCpuArray<ContextPressureStats> =
    PerCpuArray::with_max_entries(1, 0);

#[map(name = "agent_pids")]
static AGENT_PIDS: HashMap<u32, u32> = HashMap::with_max_entries(1024, 0);

#[map(name = "tracked_comms")]
static TRACKED_COMMS: HashMap<[u8; 16], u32> = HashMap::with_max_entries(256, 0);

#[map(name = "tracked_paths")]
static TRACKED_PATHS: HashMap<[u8; 256], u32> = HashMap::with_max_entries(512, 0);

#[map(name = "exit_ctx")]
static EXIT_CTX: HashMap<u64, ExitMeta> = HashMap::with_max_entries(6144, 0);

#[map(name = "exit_io_ctx")]
static EXIT_IO_CTX: HashMap<u64, ExitIoMeta> = HashMap::with_max_entries(4096, 0);

#[map(name = "exit_compact_ctx")]
static EXIT_COMPACT_CTX: HashMap<u64, ExitCompactMeta> = HashMap::with_max_entries(8192, 0);

#[map(name = "socket_fds")]
static SOCKET_FDS: LruHashMap<SocketFdKey, SocketFdMeta> = LruHashMap::with_max_entries(16384, 0);

#[map(name = "socket_fd_parents")]
static SOCKET_FD_PARENTS: LruHashMap<u32, u32> = LruHashMap::with_max_entries(8192, 0);

#[map(name = "exit_single_path_ctx")]
static EXIT_SINGLE_PATH_CTX: HashMap<u64, ExitSinglePathData> = HashMap::with_max_entries(2048, 0);

#[map(name = "exit_path_ctx")]
static EXIT_PATH_CTX: HashMap<u64, ExitPathData> = HashMap::with_max_entries(1024, 0);

#[tracepoint]
pub fn sys_enter_execve(_ctx: TracePointContext) -> u32 {
    emit_execve();
    0
}

#[inline(always)]
fn emit_execve() {
    let pid_tgid = bpf_get_current_pid_tgid();
    let uid_gid = bpf_get_current_uid_gid();

    let stats = unsafe { COLLECTOR_STATS.get_ptr_mut(0).map(|stats| &mut *stats) };

    let (sequence, audit_generation, pending_dropped, reserve_failures) = if let Some(stats) = stats
    {
        stats.event_sequence = stats.event_sequence.wrapping_add(1);
        (
            stats.event_sequence,
            stats.audit_generation,
            stats.pending_dropped_events,
            stats.ringbuf_reserve_failed_total,
        )
    } else {
        (0, 0, 0, 0)
    };

    let Some(mut slot) = EVENTS.reserve::<Event>(0) else {
        if let Some(stats) = unsafe { COLLECTOR_STATS.get_ptr_mut(0).map(|stats| &mut *stats) } {
            stats.ringbuf_reserve_failed_total = stats.ringbuf_reserve_failed_total.wrapping_add(1);
            stats.pending_dropped_events = stats.pending_dropped_events.wrapping_add(1);
        }
        return;
    };

    let event = unsafe {
        let event = slot.as_mut_ptr();
        core::ptr::write_bytes(event, 0, 1);
        &mut *event
    };

    event.pid = pid_tgid as u32;
    event.tgid = (pid_tgid >> 32) as u32;
    event.uid = uid_gid as u32;
    event.gid = (uid_gid >> 32) as u32;
    event.event_type = event_type::EXECVE;
    event.comm = bpf_get_current_comm().unwrap_or([0; 16]);
    event.cgroup_id = unsafe { bpf_get_current_cgroup_id() };
    event.kernel_timestamp_ns = unsafe { bpf_ktime_get_ns() };
    event.kernel_sequence = sequence;
    event.kernel_cpu = unsafe { bpf_get_smp_processor_id() };
    event.kernel_audit_generation = audit_generation;
    event.kernel_dropped_since_last = pending_dropped;
    event.kernel_reserve_failures_total = reserve_failures;

    if let Some(stats) = unsafe { COLLECTOR_STATS.get_ptr_mut(0).map(|stats| &mut *stats) } {
        stats.ringbuf_events_total = stats.ringbuf_events_total.wrapping_add(1);
        stats.pending_dropped_events = 0;
    }

    slot.submit(0);
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    loop {}
}
