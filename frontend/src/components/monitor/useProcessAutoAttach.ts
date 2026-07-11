import { onScopeDispose, watch, type Ref } from "vue";

interface AutoAttachProcess {
  pid: number;
}

export function useProcessAutoAttach(options: {
  pendingProcesses: Readonly<Ref<readonly AutoAttachProcess[]>>;
  enabled: Readonly<Ref<boolean>>;
  isActive: () => boolean;
  attach: (pid: number) => void | Promise<unknown>;
  staggerMs?: number;
}) {
  const {
    pendingProcesses,
    enabled,
    isActive,
    attach,
    staggerMs = 100,
  } = options;
  const attemptedPids = new Set<number>();
  const pendingTimers = new Map<number, ReturnType<typeof setTimeout>>();

  const clearPendingTimers = () => {
    for (const [pid, timer] of pendingTimers) {
      clearTimeout(timer);
      attemptedPids.delete(pid);
    }
    pendingTimers.clear();
  };

  watch(
    [pendingProcesses, enabled, isActive],
    ([pending, autoAttachEnabled, active]) => {
      if (!autoAttachEnabled || !active) {
        clearPendingTimers();
        return;
      }

      const pendingPids = new Set(pending.map((process) => process.pid));
      for (const pid of attemptedPids) {
        if (!pendingPids.has(pid) && !pendingTimers.has(pid)) {
          attemptedPids.delete(pid);
        }
      }

      for (const process of pending) {
        if (attemptedPids.has(process.pid)) continue;
        attemptedPids.add(process.pid);

        // Stagger requests so a large process tree cannot flood the backend.
        const timer = setTimeout(() => {
          pendingTimers.delete(process.pid);
          const stillPending = pendingProcesses.value.some(
            (current) => current.pid === process.pid,
          );
          if (!isActive() || !enabled.value || !stillPending) {
            attemptedPids.delete(process.pid);
            return;
          }
          void attach(process.pid);
        }, staggerMs * (pendingTimers.size + 1));
        pendingTimers.set(process.pid, timer);
      }
    },
    { deep: false, immediate: true },
  );

  onScopeDispose(clearPendingTimers);
}
