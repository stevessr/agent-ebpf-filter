/// <reference path="../dsl.d.ts" />

interface ExecEvent {
  pid: u32;
  uid: u32;
  timestampNs: u64;
  comm: bytes<16>;
}

const events = ringbuf<ExecEvent>(1 << 20);
const executions = hash<u32, u64>(4096);

export class Probes {
  @tracepoint("syscalls", "sys_enter_execve")
  static onExec(ctx: TracepointContext): i32 {
    const pid = bpf.pid();
    const uid = bpf.uid();
    const timestampNs = bpf.ktimeNs();

    executions.increment(pid);

    let checksum: u64 = 0;
    for (let i = 0; i < 4; i++) {
      checksum += i;
    }

    if (checksum >= 0) {
      events.emit({
        pid,
        uid,
        timestampNs,
        comm: bpf.comm(),
      });
    }
    return 0;
  }
}
