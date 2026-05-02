import { ref, onUnmounted } from 'vue';
import { buildWebSocketUrl } from '../utils/requestContext';

export function useMLStatusStream(onUpdate: (data: any) => void) {
  const isConnected = ref(false);
  let ws: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let shouldReconnect = true;
  let reconnectDelay = 3000;

  const clearReconnectTimer = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  };

  const scheduleReconnect = () => {
    if (!shouldReconnect) return;
    clearReconnectTimer();
    reconnectTimer = setTimeout(() => {
      reconnectDelay = Math.min(reconnectDelay * 2, 30000);
      connect();
    }, reconnectDelay);
  };

  const connect = () => {
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    try {
      ws = new WebSocket(buildWebSocketUrl('/ws/ml-status', { interval: 1000 }));

      ws.onopen = () => {
        isConnected.value = true;
        reconnectDelay = 3000;
      };

      ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          onUpdate(data);
        } catch (_) {}
      };

      ws.onclose = () => {
        isConnected.value = false;
        ws = null;
        scheduleReconnect();
      };

      ws.onerror = () => {
        ws?.close();
      };
    } catch (_) {
      scheduleReconnect();
    }
  };

  const disconnect = () => {
    shouldReconnect = false;
    clearReconnectTimer();
    if (ws) {
      ws.onclose = null;
      ws.close();
      ws = null;
    }
    isConnected.value = false;
  };

  onUnmounted(() => {
    disconnect();
  });

  return { isConnected, connect, disconnect };
}
