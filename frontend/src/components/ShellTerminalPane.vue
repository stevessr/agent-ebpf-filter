<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { message } from 'ant-design-vue';
import { WTerm } from '@wterm/dom';
import '@wterm/dom/css';

import type { ShellSessionInfo } from '../types/shell';

const INITIAL_COLS = 100;
const INITIAL_ROWS = 32;

const props = defineProps<{
  session: ShellSessionInfo;
  active?: boolean;
}>();

const emit = defineEmits<{
  (event: 'detach'): void;
  (event: 'close-session'): void;
}>();

type SocketStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error';

const terminalRef = ref<HTMLDivElement | null>(null);
const socketStatus = ref<SocketStatus>('idle');
const connectionError = ref('');
const connecting = ref(false);

let term: WTerm | null = null;
let socket: WebSocket | null = null;
let generation = 0;
let pendingResize: { cols: number; rows: number } | null = null;
let pendingInput: string[] = [];

const backendStatusColor = computed(() => {
  switch (props.session.status) {
    case 'running':
      return 'success';
    case 'exited':
      return 'warning';
    case 'closed':
      return 'default';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
});

const backendStatusLabel = computed(() => props.session.status || 'unknown');

const socketStatusColor = computed(() => {
  switch (socketStatus.value) {
    case 'connecting':
      return 'blue';
    case 'open':
      return 'success';
    case 'closed':
      return 'default';
    case 'error':
      return 'error';
    default:
      return 'default';
  }
});

const socketStatusLabel = computed(() => {
  switch (socketStatus.value) {
    case 'connecting':
      return 'Connecting';
    case 'open':
      return 'Connected';
    case 'closed':
      return 'Disconnected';
    case 'error':
      return 'Error';
    default:
      return 'Idle';
  }
});

const canReconnect = computed(() => props.session.status === 'running');

const shellLabel = computed(() => {
  const shell = props.session.shell || 'auto';
  return props.session.shellPath ? `${shell} → ${props.session.shellPath}` : shell;
});

const statusNotice = computed(() => {
  if (connectionError.value) {
    return {
      type: 'error' as const,
      message: connectionError.value,
      description: props.session.lastError ? `Last error: ${props.session.lastError}` : undefined,
    };
  }

  if (socketStatus.value === 'error') {
    return {
      type: 'error' as const,
      message: props.session.lastError || 'Terminal websocket connection failed',
      description: props.session.lastError ? `Last error: ${props.session.lastError}` : undefined,
    };
  }

  if (props.session.status !== 'running') {
    return {
      type: props.session.status === 'error' ? ('error' as const) : ('warning' as const),
      message: props.session.lastError || 'Backend terminal session is not running',
      description: props.session.lastError ? `Last error: ${props.session.lastError}` : undefined,
    };
  }

  if (socketStatus.value === 'closed') {
    return {
      type: 'warning' as const,
      message: 'Terminal connection closed. You can reconnect to the existing backend session.',
      description: props.session.lastError ? `Last error: ${props.session.lastError}` : undefined,
    };
  }

  return null;
});

const wsUrl = () => {
  const scheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const params = new URLSearchParams({
    session_id: props.session.id,
    cols: String(INITIAL_COLS),
    rows: String(INITIAL_ROWS),
  });
  return `${scheme}//${window.location.host}/ws/shell?${params.toString()}`;
};

const flushPending = () => {
  if (!socket || socket.readyState !== WebSocket.OPEN) return;

  if (pendingResize) {
    socket.send(JSON.stringify({ type: 'resize', ...pendingResize }));
    pendingResize = null;
  }

  if (pendingInput.length) {
    for (const payload of pendingInput) {
      socket.send(payload);
    }
    pendingInput = [];
  }
};

const sendShellData = (payload: string) => {
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(payload);
    return;
  }
  pendingInput.push(payload);
};

const sendResize = (cols: number, rows: number) => {
  pendingResize = { cols, rows };
  if (socket && socket.readyState === WebSocket.OPEN) {
    flushPending();
  }
};

const focusTerminal = () => {
  term?.focus();
};

const cleanup = () => {
  socket?.close();
  socket = null;
  pendingResize = null;
  pendingInput = [];
  term?.destroy();
  term = null;
};

const terminalFontSize = ref(14);
const terminalStyle = computed(() => {
  const lh = 1.2;
  return {
    '--term-font-size': `${terminalFontSize.value}px`,
    '--term-line-height': lh,
    '--term-row-height': `${Math.round(terminalFontSize.value * lh)}px`,
  };
});

const triggerTerminalResize = () => {
  if (!term || !terminalRef.value) return;
  nextTick(() => {
    const measured = (term as any)._measureCharSize();
    if (measured) {
      if (typeof (term as any)._setRowHeight === 'function') {
        (term as any)._setRowHeight();
      }
      const rect = terminalRef.value!.getBoundingClientRect();
      const newCols = Math.max(1, Math.floor(rect.width / measured.charWidth));
      const newRows = Math.max(1, Math.floor(rect.height / measured.rowHeight));
      term?.resize(newCols, newRows);
    }
  });
};

const connect = async () => {
  const currentGeneration = ++generation;
  connecting.value = true;
  socketStatus.value = 'connecting';
  connectionError.value = '';

  cleanup();
  await nextTick();

  if (!terminalRef.value || currentGeneration !== generation) {
    connecting.value = false;
    return;
  }

  terminalRef.value.innerHTML = '';

  try {
    const currentSocket = new WebSocket(wsUrl());
    socket = currentSocket;
    currentSocket.binaryType = 'arraybuffer';
    currentSocket.onopen = () => {
      if (currentGeneration !== generation) {
        currentSocket.close();
        return;
      }
      socketStatus.value = 'open';
      connecting.value = false;
      flushPending();
      focusTerminal();
    };
    currentSocket.onmessage = (event) => {
      if (currentGeneration !== generation) return;
      if (!term) return;

      if (typeof event.data === 'string') {
        term.write(event.data);
      } else if (event.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(event.data));
      }
    };
    currentSocket.onclose = () => {
      if (currentGeneration !== generation) return;
      if (socketStatus.value === 'connecting') {
        socketStatus.value = 'error';
        connectionError.value = 'Shell websocket closed before opening';
      } else if (socketStatus.value !== 'error') {
        socketStatus.value = 'closed';
      }
      connecting.value = false;
    };
    currentSocket.onerror = () => {
      if (currentGeneration !== generation) return;
      socketStatus.value = 'error';
      connecting.value = false;
      connectionError.value = 'Shell websocket connection failed';
    };

    term = new WTerm(terminalRef.value, {
      cols: INITIAL_COLS,
      rows: INITIAL_ROWS,
      autoResize: true,
      cursorBlink: true,
      onData: (data) => {
        if (currentGeneration === generation) {
          sendShellData(data);
          // WTerm natively scrolls to bottom on user input
        }
      },
      onResize: (cols, rows) => {
        if (currentGeneration === generation) {
          sendResize(cols, rows);
        }
      },
    });

    // Monkey-patch _isScrolledToBottom to workaround WTerm 0.1.9 row-height quantization bug
    // which prevents auto-scrolling when scroll position is a few pixels from the absolute bottom.
    (term as any)._isScrolledToBottom = function () {
      const el = this.element;
      if (!el) return true;
      return el.scrollHeight - el.scrollTop - el.clientHeight < 40;
    };

    await term.init();
    if (currentGeneration !== generation) {
      return;
    }

    // Monkey-patch keyToSequence to fix macOS alt+ key combinations (e.g., alt+f, alt+b)
    // macOS translates alt+letter into special characters (like ƒ, ∫). Terminal expects \x1b + letter.
    // This must be done AFTER term.init() because term.input is created during init.
    const inputHandler = (term as any).input;
    if (inputHandler && inputHandler.keyToSequence) {
      const originalKeyToSequence = inputHandler.keyToSequence.bind(inputHandler);
      inputHandler.keyToSequence = function(e: KeyboardEvent) {
        const isZoom = (e.ctrlKey || e.metaKey) && !e.altKey;
        if (isZoom) {
          if (e.code === 'Equal' || e.code === 'NumpadAdd' || e.key === '=' || e.key === '+') {
            e.preventDefault();
            terminalFontSize.value = Math.min(48, terminalFontSize.value + 1);
            triggerTerminalResize();
            return null;
          }
          if (e.code === 'Minus' || e.code === 'NumpadSubtract' || e.key === '-' || e.key === '_') {
            e.preventDefault();
            terminalFontSize.value = Math.max(8, terminalFontSize.value - 1);
            triggerTerminalResize();
            return null;
          }
          if (e.code === 'Digit0' || e.key === '0') {
            e.preventDefault();
            terminalFontSize.value = 14;
            triggerTerminalResize();
            return null;
          }
        }

        if (e.altKey && !e.ctrlKey && !e.metaKey) {
          // Handle Option+Arrows for word jumping and Option+Backspace for word deletion
          if (e.key === 'ArrowLeft') return '\x1bb';
          if (e.key === 'ArrowRight') return '\x1bf';
          if (e.key === 'ArrowUp') return '\x1b[1;3A';
          if (e.key === 'ArrowDown') return '\x1b[1;3B';
          if (e.key === 'Backspace') return '\x1b\x7f';

          if (e.key.length === 1) {
            // Use e.keyCode to get the unmodified letter/number safely
            if (e.keyCode >= 65 && e.keyCode <= 90) {
              const char = String.fromCharCode(e.keyCode + (e.shiftKey ? 0 : 32));
              return '\x1b' + char;
            }
            if (e.keyCode >= 48 && e.keyCode <= 57) {
              return '\x1b' + String.fromCharCode(e.keyCode);
            }
            // Fallback to e.code for common punctuation
            const symbolMap: Record<string, string> = {
              'Minus': '-', 'Equal': '=', 'BracketLeft': '[', 'BracketRight': ']',
              'Backslash': '\\', 'Semicolon': ';', 'Quote': "'", 'Comma': ',',
              'Period': '.', 'Slash': '/', 'Backquote': '`'
            };
            if (e.code && symbolMap[e.code]) {
              return '\x1b' + symbolMap[e.code];
            }
          }
        }
        return originalKeyToSequence(e);
      };
    }

    sendResize(term.cols, term.rows);
    flushPending();
    focusTerminal();
  } catch (err: any) {
    if (currentGeneration !== generation) return;
    socketStatus.value = 'error';
    connecting.value = false;
    connectionError.value = err?.message || 'Failed to connect to terminal session';
    message.error(connectionError.value);
    cleanup();
  }
};

const reconnect = () => {
  void connect();
};

watch(
  () => props.session.id,
  () => {
    void connect();
  },
);

watch(
  () => props.active,
  async (active) => {
    if (!active) return;
    await nextTick();
    focusTerminal();
    if (term) {
      sendResize(term.cols, term.rows);
    }
  },
);

onMounted(() => {
  void connect();
});

onBeforeUnmount(() => {
  generation += 1;
  cleanup();
});
</script>

<template>
  <div class="shell-pane">
    <div class="shell-pane__toolbar">
      <div class="shell-pane__meta">
        <div class="shell-pane__meta-line">
          <a-tag color="blue">#{{ session.id }}</a-tag>
          <span class="shell-pane__shell" :title="shellLabel">{{ shellLabel }}</span>
        </div>
        <div class="shell-pane__meta-line">
          <a-tag :color="backendStatusColor">backend: {{ backendStatusLabel }}</a-tag>
          <a-tag :color="socketStatusColor">socket: {{ socketStatusLabel }}</a-tag>
        </div>
        <div class="shell-pane__meta-line">
          <span class="shell-pane__path" :title="session.workDir">{{ session.workDir }}</span>
        </div>
      </div>

      <a-space wrap>
        <a-button size="small" :loading="connecting" :disabled="!canReconnect || connecting" @click="reconnect">
          Reconnect
        </a-button>
        <a-button size="small" @click="focusTerminal">
          Focus
        </a-button>
        <a-button size="small" @click="emit('detach')">
          Detach
        </a-button>
        <a-button size="small" danger @click="emit('close-session')">
          Close backend
        </a-button>
      </a-space>
    </div>

    <a-alert
      v-if="statusNotice"
      class="shell-pane__alert"
      :type="statusNotice.type"
      show-icon
      :message="statusNotice.message"
      :description="statusNotice.description"
    />

    <div ref="terminalRef" class="wterm theme-monokai shell-pane__terminal" :style="terminalStyle"></div>
  </div>
</template>

<style scoped>
.shell-pane {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.shell-pane__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.shell-pane__meta {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.shell-pane__meta-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex-wrap: nowrap;
}

.shell-pane__shell,
.shell-pane__path {
  color: #666;
  font-size: 13px;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.shell-pane__alert {
  margin-bottom: 0;
}

.shell-pane__terminal {
  width: 100%;
  height: 520px;
}
</style>
