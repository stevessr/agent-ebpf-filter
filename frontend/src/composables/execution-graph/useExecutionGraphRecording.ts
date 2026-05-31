import { ref, type Ref } from "vue";
import axios from "axios";
import { message } from "ant-design-vue";

import type {
  ExecutionGraphFilterState,
  ExecutionGraphResponse,
} from "../../types/executionGraph";

export type GraphState = ExecutionGraphResponse & {
  nodes: any[];
  edges: any[];
};
export type BrowserGraphSnapshot = { recordedAt: string; graph: GraphState };

export type ExecutionGraphRecordingDeps = {
  graph: Ref<GraphState>;
  filters: ExecutionGraphFilterState;
  replayPath: Ref<string>;
  browserSnapshots: Ref<BrowserGraphSnapshot[]>;
  browserRecordingActive: Ref<boolean>;
  browserReplayActive: Ref<boolean>;
  applyGraphPayload: (
    payload: Partial<ExecutionGraphResponse> | undefined,
  ) => void;
  syncRouteQuery: () => Promise<void>;
  connectGraphSocket: () => void;
  closeGraphSocket: (
    status?: "connecting" | "connected" | "paused" | "closed" | "error",
  ) => void;
  cloneGraphState: (graph: GraphState) => GraphState;
  appendBrowserSnapshot: (graph: GraphState) => void;
  setLastLoadedAt: (value: string) => void;
  browserSnapshotCount: Ref<number>;
  replayEnabled: Ref<boolean>;
  browserRecordingSummary: Ref<string>;
};

export function useExecutionGraphRecording(deps: ExecutionGraphRecordingDeps) {
  // ── File recording state ──
  const recordingPath = ref("");
  const recordingActive = ref(false);
  const recordingCount = ref(0);
  const recordingStartedAt = ref("");
  const recordingBusy = ref(false);
  const replayBusy = ref(false);
  let recordingStatusTimer: ReturnType<typeof setInterval> | null = null;

  // ── Browser recording state ──
  const browserReplayIndex = ref(0);
  const browserSavePath = ref("");
  const browserSaveBusy = ref(false);
  let browserReplayTimer: ReturnType<typeof setInterval> | null = null;

  // ── File recording functions ──

  const loadRecordingStatus = async () => {
    try {
      const { data } = await axios.get("/events/recording");
      recordingActive.value = Boolean(data?.active);
      recordingCount.value = Number(data?.count ?? 0);
      recordingStartedAt.value = String(data?.startedAt ?? "");
      if (!recordingPath.value) {
        recordingPath.value = String(data?.path || data?.defaultPath || "");
      }
    } catch (error) {
      console.error("Failed to load event recording status", error);
    }
  };

  const startRecording = async () => {
    recordingBusy.value = true;
    try {
      const { data } = await axios.post("/events/recording/start", {
        path: recordingPath.value,
        truncate: false,
      });
      recordingActive.value = Boolean(data?.active);
      recordingCount.value = Number(data?.count ?? 0);
      recordingStartedAt.value = String(data?.startedAt ?? "");
      recordingPath.value = String(data?.path || recordingPath.value);
      message.success("已开始录制事件到文件");
    } catch (error) {
      console.error("Failed to start event recording", error);
      message.error("开始录制失败");
    } finally {
      recordingBusy.value = false;
    }
  };

  const stopRecording = async () => {
    recordingBusy.value = true;
    try {
      const { data } = await axios.post("/events/recording/stop");
      recordingActive.value = Boolean(data?.active);
      recordingCount.value = Number(data?.count ?? recordingCount.value);
      message.success("已停止录制");
    } catch (error) {
      console.error("Failed to stop event recording", error);
      message.error("停止录制失败");
    } finally {
      recordingBusy.value = false;
    }
  };

  const playRecording = async () => {
    const path = recordingPath.value.trim();
    if (!path) {
      message.warning("请先填写录制文件路径");
      return;
    }
    replayBusy.value = true;
    try {
      const { data } = await axios.post("/events/recording/replay", {
        path,
        limit: deps.filters.limit,
      });
      deps.replayPath.value = String(data?.path || path);
      deps.applyGraphPayload(data?.graph);
      await deps.syncRouteQuery();
      deps.connectGraphSocket();
      message.success(`已回放 ${Number(data?.events ?? 0)} 条事件`);
    } catch (error) {
      console.error("Failed to replay event recording", error);
      message.error("回放录制文件失败");
    } finally {
      replayBusy.value = false;
    }
  };

  const stopReplay = async () => {
    deps.replayPath.value = "";
    await deps.syncRouteQuery();
    deps.connectGraphSocket();
  };

  // ── Browser recording functions ──

  const stopBrowserReplay = () => {
    if (browserReplayTimer) {
      clearInterval(browserReplayTimer);
      browserReplayTimer = null;
    }
    deps.browserReplayActive.value = false;
    browserReplayIndex.value = 0;
  };

  const startBrowserRecording = () => {
    stopBrowserReplay();
    deps.browserSnapshots.value = [];
    deps.browserRecordingActive.value = true;
    deps.appendBrowserSnapshot(deps.graph.value);
    message.success("已开始录制到浏览器内存");
  };

  const stopBrowserRecording = () => {
    deps.browserRecordingActive.value = false;
    message.success(
      `已停止内存录制，共 ${deps.browserSnapshotCount.value} 个快照`,
    );
  };

  const playBrowserRecording = () => {
    if (!deps.browserSnapshots.value.length) {
      message.warning("浏览器内存中没有可回放的快照");
      return;
    }
    deps.closeGraphSocket("paused");
    deps.browserRecordingActive.value = false;
    deps.browserReplayActive.value = true;
    browserReplayIndex.value = 0;
    const snapshots = deps.browserSnapshots.value;
    const playNext = () => {
      const snapshot = snapshots[browserReplayIndex.value];
      if (!snapshot) {
        stopBrowserReplay();
        return;
      }
      deps.applyGraphPayload({
        ...deps.cloneGraphState(snapshot.graph),
        source: "browser_memory",
      });
      deps.setLastLoadedAt(snapshot.recordedAt);
      browserReplayIndex.value += 1;
      if (browserReplayIndex.value >= snapshots.length) {
        stopBrowserReplay();
      }
    };
    playNext();
    browserReplayTimer = setInterval(playNext, 900);
  };

  const clearBrowserRecording = () => {
    stopBrowserReplay();
    deps.browserRecordingActive.value = false;
    deps.browserSnapshots.value = [];
    message.success("已清空浏览器内存录制");
  };

  const exitBrowserReplay = () => {
    stopBrowserReplay();
    deps.connectGraphSocket();
  };

  // ── Browser recording export/save ──

  const buildBrowserRecordingExport = () => ({
    version: 1,
    kind: "agent-ebpf-filter.execution-graph.browser-memory",
    exportedAt: new Date().toISOString(),
    snapshotCount: deps.browserSnapshots.value.length,
    snapshots: deps.browserSnapshots.value.map((snapshot) => ({
      recordedAt: snapshot.recordedAt,
      graph: deps.cloneGraphState(snapshot.graph),
    })),
  });

  const browserRecordingFilename = () => {
    const stamp = new Date().toISOString().replace(/[:.]/g, "-");
    return `execution-graph-browser-memory-${stamp}.json`;
  };

  const exportBrowserRecording = () => {
    if (!deps.browserSnapshots.value.length) {
      message.warning("浏览器内存中没有可导出的快照");
      return;
    }
    const payload = JSON.stringify(buildBrowserRecordingExport(), null, 2);
    const blob = new Blob([payload, "\n"], {
      type: "application/json;charset=utf-8",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = browserRecordingFilename();
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    message.success("已导出浏览器内存录制");
  };

  const saveBrowserRecordingToBackend = async () => {
    if (!deps.browserSnapshots.value.length) {
      message.warning("浏览器内存中没有可保存的快照");
      return;
    }
    browserSaveBusy.value = true;
    try {
      const { data } = await axios.post("/events/recording/browser/save", {
        path: browserSavePath.value.trim(),
        export: buildBrowserRecordingExport(),
      });
      browserSavePath.value = String(data?.path || browserSavePath.value);
      message.success(`已保存到后端：${browserSavePath.value}`);
    } catch (error) {
      console.error("Failed to save browser recording to backend", error);
      message.error("保存浏览器内存录制到后端失败");
    } finally {
      browserSaveBusy.value = false;
    }
  };

  // ── Lifecycle ──

  const startRecordingStatusPolling = () => {
    void loadRecordingStatus();
    recordingStatusTimer = setInterval(() => {
      void loadRecordingStatus();
    }, 2500);
  };

  const stopRecordingStatusPolling = () => {
    if (recordingStatusTimer) {
      clearInterval(recordingStatusTimer);
      recordingStatusTimer = null;
    }
  };

  const cleanup = () => {
    stopRecordingStatusPolling();
    stopBrowserReplay();
  };

  return {
    // File recording
    recordingPath,
    recordingActive,
    recordingCount,
    recordingStartedAt,
    recordingBusy,
    replayBusy,
    loadRecordingStatus,
    startRecording,
    stopRecording,
    playRecording,
    stopReplay,
    // Browser recording
    browserReplayIndex,
    browserSavePath,
    browserSaveBusy,
    startBrowserRecording,
    stopBrowserRecording,
    playBrowserRecording,
    clearBrowserRecording,
    exitBrowserReplay,
    exportBrowserRecording,
    saveBrowserRecordingToBackend,
    // Lifecycle
    startRecordingStatusPolling,
    stopRecordingStatusPolling,
    cleanup,
  };
}
