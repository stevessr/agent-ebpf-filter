import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import axios from 'axios';
import { message } from 'ant-design-vue';
import { buildWebSocketUrl } from '../../utils/requestContext';
import type {
  ShellConfig,
  ShellMode,
  ShellSessionCreateRequest,
  ShellSessionInfo,
  ShellSessionInputRequest,
} from '../../types/shell';
import { isTmuxSession, TMUX_SHORTCUTS } from '../../utils/tmux';

const SHELL_STORAGE_KEY = 'executor-shell-config';

const normalizeShellMode = (value: unknown): ShellMode => {
  const candidate = String(value || '').trim().toLowerCase();
  if (
    candidate === 'auto' ||
    candidate === 'system' ||
    candidate === 'env' ||
    candidate === 'fish' ||
    candidate === 'zsh' ||
    candidate === 'bash' ||
    candidate === 'ash' ||
    candidate === 'sh' ||
    candidate === 'custom'
  ) {
    return candidate === 'env' ? 'system' : (candidate as ShellMode);
  }
  return 'auto';
};

const loadShellConfig = (): ShellConfig => {
  try {
    const parsed = JSON.parse(localStorage.getItem(SHELL_STORAGE_KEY) || '{}') as Partial<ShellConfig>;
    return {
      mode: normalizeShellMode(parsed.mode),
      customPath: typeof parsed.customPath === 'string' ? parsed.customPath : '',
    };
  } catch {
    return { mode: 'auto', customPath: '' };
  }
};

export const SHELL_MODE_OPTIONS = [
  { label: 'Auto (fish → zsh → bash → ash → sh)', value: 'auto' },
  { label: 'System shell ($SHELL)', value: 'system' },
  { label: 'fish', value: 'fish' },
  { label: 'zsh', value: 'zsh' },
  { label: 'bash', value: 'bash' },
  { label: 'ash', value: 'ash' },
  { label: 'sh', value: 'sh' },
  { label: 'Custom path', value: 'custom' },
] as const;

export function useShellSessions(sessionKindFilter: 'all' | 'tmux' | 'non-tmux' = 'all') {
  const initialShellConfig = loadShellConfig();
  const defaultShellMode = ref<ShellMode>(initialShellConfig.mode);
  const defaultCustomShellPath = ref(initialShellConfig.customPath);

  const sessions = ref<ShellSessionInfo[]>([]);
  const sessionsLoading = ref(false);
  const sessionError = ref('');
  const creating = ref(false);

  const openSessionIds = ref<string[]>([]);
  const activeTabKey = ref('');

  const wsConnected = ref(false);
  let ws: WebSocket | null = null;
  let wsReconnectTimer: number | null = null;
  let shouldReconnect = true;

  const isTmuxFilteredView = computed(() => sessionKindFilter === 'tmux');
  const isNonTmuxFilteredView = computed(() => sessionKindFilter === 'non-tmux');

  const persistShellConfig = () => {
    const payload: ShellConfig = {
      mode: defaultShellMode.value,
      customPath: defaultCustomShellPath.value,
    };
    localStorage.setItem(SHELL_STORAGE_KEY, JSON.stringify(payload));
  };

  watch([defaultShellMode, defaultCustomShellPath], persistShellConfig, { immediate: true });

  const matchesSessionFilter = (session: ShellSessionInfo) => {
    const kind = (session.kind || '').trim().toLowerCase();
    if (sessionKindFilter === 'all') return true;
    if (sessionKindFilter === 'tmux') return isTmuxSession(session);
    if (sessionKindFilter === 'non-tmux') return !isTmuxSession(session) && kind !== 'wrapper';
    return true;
  };

  const filteredSessions = computed(() => sessions.value.filter(matchesSessionFilter));
  const tmuxQuickShortcuts = TMUX_SHORTCUTS.filter((shortcut) => !shortcut.danger);

  const defaultShellRequest = computed(() => {
    if (defaultShellMode.value === 'custom') {
      return defaultCustomShellPath.value.trim();
    }
    return defaultShellMode.value;
  });

  const canCreateSession = computed(() => {
    if (defaultShellMode.value === 'custom') {
      return defaultShellRequest.value.length > 0;
    }
    return true;
  });

  const shellSelectionLabel = computed(() => {
    switch (defaultShellMode.value) {
      case 'auto':
        return 'Auto: fish → zsh → bash → ash → sh';
      case 'system':
        return 'System: $SHELL';
      case 'custom':
        return defaultShellRequest.value ? `Custom: ${defaultShellRequest.value}` : 'Custom: unset';
      default:
        return defaultShellMode.value;
    }
  });

  const sessionMap = computed(() => new Map(sessions.value.map((session) => [session.id, session] as const)));
  const filteredSessionMap = computed(
    () => new Map(filteredSessions.value.map((session) => [session.id, session] as const)),
  );

  const sessionColumns = [
    { title: 'Session', dataIndex: 'session', key: 'session' },
    { title: 'PID', dataIndex: 'pid', key: 'pid' },
    { title: 'Status', dataIndex: 'status', key: 'status' },
    { title: 'Updated', dataIndex: 'updatedAt', key: 'updatedAt' },
    { title: 'Actions', dataIndex: 'actions', key: 'actions' },
  ];

  const openSessions = computed(() =>
    openSessionIds.value
      .map((id) => filteredSessionMap.value.get(id))
      .filter((session): session is ShellSessionInfo => Boolean(session)),
  );

  const runningSessionCount = computed(
    () => filteredSessions.value.filter((session) => session.status === 'running').length,
  );

  const formatDateTime = (value: string) => {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString();
  };

  const shellStatusColor = (status: string) => {
    switch (status) {
      case 'running': return 'success';
      case 'exited': return 'warning';
      case 'closed': return 'default';
      case 'error': return 'error';
      default: return 'default';
    }
  };

  const attachedColor = (attached: boolean) => (attached ? 'success' : 'default');

  const isSessionOpen = (sessionId: string) => openSessionIds.value.includes(sessionId);

  const syncOpenTabs = () => {
    const availableIds = new Set(filteredSessions.value.map((session) => session.id));
    openSessionIds.value = openSessionIds.value.filter((id) => availableIds.has(id));
    if (!openSessionIds.value.includes(activeTabKey.value)) {
      activeTabKey.value = openSessionIds.value[0] || '';
    }
  };

  const upsertSession = (session: ShellSessionInfo) => {
    const index = sessions.value.findIndex((item) => item.id === session.id);
    if (index >= 0) {
      sessions.value = sessions.value.map((item) => (item.id === session.id ? session : item));
    } else {
      sessions.value = [session, ...sessions.value];
    }
    syncOpenTabs();
  };

  const removeSessionLocally = (sessionId: string) => {
    sessions.value = sessions.value.filter((session) => session.id !== sessionId);
    openSessionIds.value = openSessionIds.value.filter((id) => id !== sessionId);
    if (activeTabKey.value === sessionId) {
      activeTabKey.value = openSessionIds.value[0] || '';
    }
  };

  const refreshSessions = async () => {
    if (sessionsLoading.value) return;

    sessionsLoading.value = true;
    sessionError.value = '';
    try {
      const res = await axios.get('/shell-sessions');
      sessions.value = Array.isArray(res.data) ? (res.data as ShellSessionInfo[]) : [];
      syncOpenTabs();
    } catch (err: any) {
      sessionError.value = err?.response?.data?.error || err?.message || 'Failed to load shell sessions';
    } finally {
      sessionsLoading.value = false;
    }
  };

  const openSession = (sessionId: string) => {
    if (!sessionId) return;
    const session = sessionMap.value.get(sessionId);
    if (!session || !matchesSessionFilter(session)) return;
    if (!openSessionIds.value.includes(sessionId)) {
      openSessionIds.value = [...openSessionIds.value, sessionId];
    }
    activeTabKey.value = sessionId;
  };

  const focusOrOpenSession = (session: ShellSessionInfo) => {
    if (isSessionOpen(session.id)) {
      activeTabKey.value = session.id;
      return;
    }

    if (session.attached) {
      message.warning(`Session #${session.id} is already attached elsewhere`);
      return;
    }

    if (session.status !== 'running') {
      message.warning(`Session #${session.id} is not running`);
      return;
    }

    openSession(session.id);
  };

  const detachSession = (sessionId: string) => {
    if (!isSessionOpen(sessionId)) return;
    openSessionIds.value = openSessionIds.value.filter((id) => id !== sessionId);
    if (activeTabKey.value === sessionId) {
      activeTabKey.value = openSessionIds.value[0] || '';
    }
  };

  const handleTabEdit = (targetKey: string | number | MouseEvent, action: 'add' | 'remove') => {
    if (action !== 'remove') return;
    detachSession(String(targetKey));
  };

  const closeBackendSession = async (sessionId: string) => {
    try {
      detachSession(sessionId);
      await axios.delete(`/shell-sessions/${sessionId}`);
      removeSessionLocally(sessionId);
      message.success(`Closed session #${sessionId}`);
    } catch (err: any) {
      message.error(err?.response?.data?.error || err?.message || 'Failed to close session');
    }
  };

  const sendSessionInput = async (sessionId: string, data: string) => {
    const payload: ShellSessionInputRequest = { data };
    await axios.post(`/shell-sessions/${sessionId}/input`, payload);
  };

  const sendTmuxShortcut = async (sessionId: string, shortcut: string, label: string) => {
    try {
      await sendSessionInput(sessionId, shortcut);
    } catch (err: any) {
      message.error(err?.response?.data?.error || err?.message || `Failed to send ${label}`);
    }
  };

  const createSession = async (defaultEnv?: Record<string, string>) => {
    if (!canCreateSession.value) {
      message.error('Please provide a custom shell path');
      return;
    }

    creating.value = true;
    try {
      const env = defaultEnv && Object.keys(defaultEnv).length > 0
        ? { ...defaultEnv }
        : undefined;
      const payload: ShellSessionCreateRequest = {
        shell: defaultShellRequest.value || 'auto',
        cols: 100,
        rows: 32,
        kind: 'shell',
        env,
      };
      const res = await axios.post('/shell-sessions', payload);
      const session = res.data as ShellSessionInfo;
      upsertSession(session);
      openSession(session.id);
      message.success(`Created shell session #${session.id}`);
      return session;
    } catch (err: any) {
      message.error(err?.response?.data?.error || err?.message || 'Failed to create session');
    } finally {
      creating.value = false;
    }
  };

  const cleanupSessions = async () => {
    try {
      await axios.post('/shell-sessions/cleanup');
      message.success('Exited/closed sessions cleaned up');
    } catch (err: any) {
      message.error(err?.response?.data?.error || 'Failed to clean up sessions');
    }
  };

  const connectShellSessionsWS = () => {
    if (!shouldReconnect) return;
    if (ws) {
      ws.close();
      ws = null;
    }

    ws = new WebSocket(buildWebSocketUrl('/ws/shell-sessions'));

    ws.onopen = () => {
      wsConnected.value = true;
      sessionError.value = '';
    };

    ws.onmessage = (message) => {
      try {
        const data = JSON.parse(message.data) as ShellSessionInfo[];
        sessions.value = Array.isArray(data) ? data : [];
        syncOpenTabs();
      } catch (err) {
        console.error('Failed to parse shell sessions update', err);
      }
    };

    ws.onerror = () => {
      wsConnected.value = false;
      sessionError.value = 'Shell sessions WebSocket 连接失败，请确认访问令牌有效且 shell_sessions 功能已启用。';
    };

    ws.onclose = () => {
      wsConnected.value = false;
      ws = null;
      if (!shouldReconnect) return;
      if (wsReconnectTimer !== null) clearTimeout(wsReconnectTimer);
      wsReconnectTimer = window.setTimeout(connectShellSessionsWS, 3000);
    };
  };

  const disconnectShellSessionsWS = () => {
    shouldReconnect = false;
    if (wsReconnectTimer !== null) {
      clearTimeout(wsReconnectTimer);
      wsReconnectTimer = null;
    }
    if (ws) {
      ws.close();
      ws = null;
    }
  };

  const sessionLabel = (session: ShellSessionInfo) => session.label || session.shell || 'auto';
  const tabLabel = (session: ShellSessionInfo) => `#${session.id} · ${sessionLabel(session)}`;

  return {
    defaultShellMode,
    defaultCustomShellPath,
    sessions,
    sessionsLoading,
    sessionError,
    creating,
    openSessionIds,
    activeTabKey,
    wsConnected,
    isTmuxFilteredView,
    isNonTmuxFilteredView,
    filteredSessions,
    tmuxQuickShortcuts,
    defaultShellRequest,
    canCreateSession,
    shellSelectionLabel,
    sessionMap,
    filteredSessionMap,
    sessionColumns,
    openSessions,
    runningSessionCount,
    formatDateTime,
    shellStatusColor,
    attachedColor,
    isSessionOpen,
    syncOpenTabs,
    upsertSession,
    removeSessionLocally,
    refreshSessions,
    openSession,
    focusOrOpenSession,
    detachSession,
    handleTabEdit,
    closeBackendSession,
    sendSessionInput,
    sendTmuxShortcut,
    createSession,
    cleanupSessions,
    connectShellSessionsWS,
    disconnectShellSessionsWS,
    sessionLabel,
    tabLabel,
  };
}
