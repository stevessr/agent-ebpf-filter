/// <reference path="../dsl.d.ts" />

// Minimal kernel BTF projection. Only fields referenced through bpf.coreRead
// need to be declared; libbpf resolves their real offsets from target BTF.
interface task_struct {
  pid: i32;
  tgid: i32;
}

interface TaskEvent {
  pid: i32;
  tgid: i32;
  currentTask: u64;
}

const taskEvents = ringbuf<TaskEvent>(1 << 16);

export class TaskProbes {
  @kprobe("wake_up_new_task")
  static wakeNewTask(ctx: KProbeContext): i32 {
    const task = bpf.arg(ctx, 1);
    taskEvents.emit({
      pid: bpf.coreRead.task_struct.pid(task),
      tgid: bpf.coreRead.task_struct.tgid(task),
      currentTask: bpf.currentTask(),
    });
    return 0;
  }
}
