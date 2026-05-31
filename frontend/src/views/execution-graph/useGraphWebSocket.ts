import { ref, watch, onUnmounted } from "vue";
import type { ExecutionGraphResponse } from "../../types/executionGraph";
import { buildWebSocketUrl } from "../../utils/requestContext";

export interface UseGraphWebSocketOptions {
  liveListen: { value: boolean };
  buildParams: () => Record<string, unknown>;
  applyGraphPayload: (
    payload: Partial<ExecutionGraphResponse> | undefined,
  ) => void;
}

export function useGraphWebSocket(opts: UseGraphWebSocketOptions) {
  const loading = ref(false);
  const graphSocketStatus = ref<
    "connecting" | "connected" | "paused" | "closed" | "error"
  >("closed");

  let graphWs: WebSocket | null = null;
  let graphReconnectTimer: ReturnType<typeof setTimeout> | null = null;

  const closeGraphSocket = (
    status: typeof graphSocketStatus.value = "closed",
  ) => {
    if (graphReconnectTimer) {
      clearTimeout(graphReconnectTimer);
      graphReconnectTimer = null;
    }
    if (graphWs) {
      const socket = graphWs;
      graphWs = null;
      socket.onopen = null;
      socket.onmessage = null;
      socket.onerror = null;
      socket.onclose = null;
      socket.close();
    }
    loading.value = false;
    graphSocketStatus.value = status;
  };

  const connectGraphSocket = () => {
    if (!opts.liveListen.value) {
      closeGraphSocket("paused");
      return;
    }
    if (graphReconnectTimer) {
      clearTimeout(graphReconnectTimer);
      graphReconnectTimer = null;
    }
    if (graphWs) {
      graphWs.onclose = null;
      graphWs.close();
      graphWs = null;
    }
    loading.value = true;
    graphSocketStatus.value = "connecting";
    const socket = new WebSocket(
      buildWebSocketUrl("/ws/events/graph", {
        ...opts.buildParams(),
        interval: 1500,
      }),
    );
    graphWs = socket;
    socket.onopen = () => {
      graphSocketStatus.value = "connected";
    };
    socket.onmessage = (event) => {
      try {
        const payload = JSON.parse(String(event.data));
        if (payload?.error) {
          throw new Error(String(payload.error));
        }
        opts.applyGraphPayload(payload);
        loading.value = false;
      } catch (error) {
        console.error(
          "Failed to parse execution graph websocket payload",
          error,
        );
        graphSocketStatus.value = "error";
        loading.value = false;
      }
    };
    socket.onerror = () => {
      graphSocketStatus.value = "error";
      loading.value = false;
    };
    socket.onclose = () => {
      if (graphWs !== socket) return;
      graphWs = null;
      if (!opts.liveListen.value) {
        graphSocketStatus.value = "paused";
        loading.value = false;
        return;
      }
      graphSocketStatus.value = "closed";
      loading.value = false;
      graphReconnectTimer = setTimeout(() => connectGraphSocket(), 2000);
    };
  };

  watch(
    () => opts.liveListen.value,
    (enabled) => {
      if (enabled) {
        connectGraphSocket();
      } else {
        closeGraphSocket("paused");
      }
    },
  );

  onUnmounted(() => {
    closeGraphSocket("closed");
  });

  return {
    loading,
    graphSocketStatus,
    connectGraphSocket,
    closeGraphSocket,
  };
}
