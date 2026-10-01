#![no_std]
#![no_main]

use aya_ebpf::{
    helpers::{bpf_get_current_pid_tgid, bpf_ktime_get_ns},
    macros::{map, tracepoint},
    maps::RingBuf,
    programs::TracePointContext,
};

#[repr(C)]
#[derive(Clone, Copy)]
pub struct BootstrapEvent {
    pub pid: u32,
    pub tid: u32,
    pub timestamp_ns: u64,
    pub event_type: u32,
    pub reserved: u32,
}

#[map]
static EVENTS: RingBuf = RingBuf::with_byte_size(256 * 1024, 0);

#[tracepoint(category = "syscalls", name = "sys_enter_execve")]
pub fn sys_enter_execve(_ctx: TracePointContext) -> u32 {
    let pid_tgid = unsafe { bpf_get_current_pid_tgid() };
    if let Some(mut slot) = EVENTS.reserve::<BootstrapEvent>(0) {
        slot.write(BootstrapEvent {
            pid: (pid_tgid >> 32) as u32,
            tid: pid_tgid as u32,
            timestamp_ns: unsafe { bpf_ktime_get_ns() },
            event_type: 1,
            reserved: 0,
        });
        slot.submit(0);
    }
    0
}

#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    loop {}
}
