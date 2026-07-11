<script setup lang="ts">
import { PlayCircleOutlined } from "@ant-design/icons-vue";
import type { useExecutionGraphRecording } from "../../composables/execution-graph/useExecutionGraphRecording";

const props = defineProps<{
  recording: ReturnType<typeof useExecutionGraphRecording>;
  browserRecordingActive: boolean;
  browserReplayActive: boolean;
  browserSnapshotCount: number;
  browserRecordingSummary: string;
  replayEnabled: boolean;
}>();
const {
  recordingPath,
  recordingActive,
  recordingCount,
  recordingStartedAt,
  recordingBusy,
  replayBusy,
  startRecording,
  stopRecording,
  playRecording,
  stopReplay,
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
} = props.recording;
</script>

<template>
<a-card :bordered="false" class="recording-card">
          <template #title
            ><span><PlayCircleOutlined /> 录制 / 回放</span></template
          >
          <a-row :gutter="12" align="middle">
            <a-col :xs="24" :lg="12">
              <a-input
                v-model:value="recordingPath"
                allow-clear
                placeholder="~/.config/agent-ebpf-filter/recordings/events.jsonl"
              />
            </a-col>
            <a-col :xs="24" :lg="12">
              <a-space wrap>
                <a-button
                  type="primary"
                  :loading="recordingBusy"
                  :disabled="recordingActive"
                  @click="startRecording"
                  >开始录制到文件</a-button
                >
                <a-button
                  danger
                  :loading="recordingBusy"
                  :disabled="!recordingActive"
                  @click="stopRecording"
                  >停止录制</a-button
                >
                <a-button :loading="replayBusy" @click="playRecording"
                  >回放文件</a-button
                >
                <a-button v-if="replayEnabled" @click="stopReplay"
                  >退出回放</a-button
                >
                <a-tag v-if="recordingActive" color="red"
                  >录制中 · {{ recordingCount }}</a-tag
                >
                <a-tag v-if="replayEnabled" color="purple">回放中</a-tag>
              </a-space>
            </a-col>
          </a-row>
          <a-typography-text
            v-if="recordingStartedAt"
            type="secondary"
            class="recording-meta"
          >
            started {{ recordingStartedAt }}
          </a-typography-text>
          <div class="browser-recording-row">
            <a-space wrap>
              <a-button
                type="primary"
                ghost
                :disabled="browserRecordingActive"
                @click="startBrowserRecording"
                >开始录制到浏览器内存</a-button
              >
              <a-button
                :disabled="!browserRecordingActive"
                @click="stopBrowserRecording"
                >停止内存录制</a-button
              >
              <a-button
                :disabled="!browserSnapshotCount"
                @click="playBrowserRecording"
                >回放内存</a-button
              >
              <a-button v-if="browserReplayActive" @click="exitBrowserReplay"
                >退出内存回放</a-button
              >
              <a-button
                :disabled="!browserSnapshotCount"
                danger
                ghost
                @click="clearBrowserRecording"
                >清空内存</a-button
              >
              <a-button
                :disabled="!browserSnapshotCount"
                @click="exportBrowserRecording"
                >导出内存 JSON</a-button
              >
              <a-button
                type="primary"
                :loading="browserSaveBusy"
                :disabled="!browserSnapshotCount"
                @click="saveBrowserRecordingToBackend"
                >保存到后端</a-button
              >
              <a-tag v-if="browserRecordingActive" color="blue"
                >内存录制中 · {{ browserSnapshotCount }}</a-tag
              >
              <a-tag v-if="browserReplayActive" color="purple"
                >内存回放 {{ browserReplayIndex }}/{{
                  browserSnapshotCount
                }}</a-tag
              >
            </a-space>
            <a-input
              v-model:value="browserSavePath"
              allow-clear
              class="browser-save-path"
              placeholder="后端保存路径，可空；默认保存到 ~/.config/agent-ebpf-filter/recordings/browser-memory-*.json"
            />
            <a-typography-text type="secondary" class="recording-meta">
              {{ browserRecordingSummary }}
            </a-typography-text>
          </div>
        </a-card>
</template>

<style scoped>
.recording-card {
  border-radius: 14px;
}

.recording-meta {
  display: block;
  margin-top: 8px;
}

.browser-recording-row {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid #f1f5f9;
}

.browser-save-path {
  margin-top: 10px;
  max-width: 780px;
}
</style>
