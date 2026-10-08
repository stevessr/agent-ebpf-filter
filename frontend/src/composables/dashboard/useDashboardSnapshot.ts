import { onBeforeUnmount, shallowRef, watch } from "vue";
import type { AgentEvent } from "./dashboardConstants";

// Throttle (not debounce): continuous traffic still publishes once per second.
// Keep the latest immutable buffer reference; don't clone or deep-watch events.
export function useDashboardSnapshot(source: () => AgentEvent[]) {
  const events = shallowRef<AgentEvent[]>([]);
  const updatedAt = shallowRef(0);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const publish = () => {
    events.value = source();
    updatedAt.value = Date.now();
  };
  watch(
    source,
    () => {
      if (!source().length) {
        if (timer) clearTimeout(timer);
        timer = undefined;
        publish();
        return;
      }
      if (timer) return;
      timer = setTimeout(() => {
        timer = undefined;
        publish();
      }, 1000);
    },
    { immediate: true },
  );
  onBeforeUnmount(() => {
    if (timer) clearTimeout(timer);
  });
  return { events, updatedAt };
}
