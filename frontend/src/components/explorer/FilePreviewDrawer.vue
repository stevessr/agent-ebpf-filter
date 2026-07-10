<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  ref,
  shallowRef,
  triggerRef,
  watch,
} from "vue";
import type { FilePreviewResponse } from "../../types/filePreview";
import { buildRequestHeaders } from "../../utils/requestContext";

type CodeHighlightingModule = typeof import("../../utils/codeHighlighting");

let codeHighlightingModulePromise: Promise<CodeHighlightingModule> | null = null;
const loadCodeHighlightingModule = () => {
  if (!codeHighlightingModulePromise) {
    codeHighlightingModulePromise = import("../../utils/codeHighlighting").catch(
      (error) => {
        codeHighlightingModulePromise = null;
        throw error;
      },
    );
  }
  return codeHighlightingModulePromise;
};

const props = withDefaults(
  defineProps<{
    open: boolean;
    loading?: boolean;
    preview: FilePreviewResponse | null;
    title?: string;
  }>(),
  {
    loading: false,
    title: "File Preview",
  },
);

const emit = defineEmits<{
  (event: "update:open", value: boolean): void;
}>();

const STREAM_CHUNK_FLUSH_LINES = 80;
const VIRTUAL_LINE_HEIGHT = 21;
const VIRTUAL_OVERSCAN_LINES = 40;
const VIRTUAL_DEFAULT_VIEWPORT_HEIGHT = 520;
const HEX_PAGE_SIZE = 4096;

type PreviewPanel = "preview" | "hex";

interface HexRow {
  offset: number;
  hex: string[];
  ascii: string;
}

interface HexResponse {
  size: number;
  offset: number;
  limit: number;
  bytesRead: number;
  nextOffset: number;
  eof: boolean;
  rows: HexRow[];
}

interface ELFAnalysis {
  header: Record<string, string | number>;
  programs: Array<Record<string, string | number>>;
  sections: Array<Record<string, string | number>>;
  dynamicLibraries: string[];
  dynamicSymbols: Array<Record<string, string | number>>;
  staticSymbols: Array<Record<string, string | number>>;
  dynamicSymbolCount: number;
  staticSymbolCount: number;
  disassembly: string;
  disassemblyError: string;
}

const drawerOpen = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value),
});

const formatBytes = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const base = 1024;
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(base)),
    units.length - 1,
  );
  return `${(bytes / Math.pow(base, index)).toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
};

const formatHexOffset = (offset: number) =>
  `0x${Math.max(0, offset).toString(16).padStart(8, "0")}`;
const objectEntries = (value?: Record<string, string | number>) =>
  Object.entries(value || {});

const formattedModTime = computed(() => {
  if (!props.preview?.modTime) return "—";
  const date = new Date(props.preview.modTime);
  return Number.isNaN(date.getTime())
    ? props.preview.modTime
    : date.toLocaleString();
});

const highlightedHtml = ref("");
const highlightLoading = ref(false);
const wordWrap = ref(true);
const activePanel = ref<PreviewPanel>("preview");
const streamLoading = ref(false);
const streamError = ref("");
const streamBytesLoaded = ref(0);
const streamLines = shallowRef<string[]>([]);
const virtualScrollTop = ref(0);
const virtualViewportHeight = ref(VIRTUAL_DEFAULT_VIEWPORT_HEIGHT);
const virtualScrollerRef = ref<HTMLElement | null>(null);
const hexLoading = ref(false);
const hexError = ref("");
const hexData = ref<HexResponse | null>(null);
const hexOffsetInput = ref(0);
const elfLoading = ref(false);
const elfError = ref("");
const elfData = ref<ELFAnalysis | null>(null);
let streamAbortController: AbortController | null = null;
let streamLineCarry = "";
let pendingStreamLineCount = 0;
let highlightRunId = 0;

const isStreamingText = computed(
  () =>
    props.open &&
    props.preview?.previewType === "text" &&
    props.preview.streamable === true,
);
const canShowHex = computed(
  () => props.preview?.hexable === true && !props.preview?.isDir,
);
const isHexPanel = computed(() => activePanel.value === "hex");
const streamProgressPercent = computed(() => {
  const size = props.preview?.size || 0;
  if (!size) return 0;
  return Math.min(100, Math.round((streamBytesLoaded.value / size) * 100));
});
const virtualTotalLines = computed(() => streamLines.value.length);
const virtualTotalHeight = computed(() =>
  Math.max(
    virtualTotalLines.value * VIRTUAL_LINE_HEIGHT,
    virtualViewportHeight.value,
  ),
);
const virtualStartLine = computed(() =>
  Math.max(
    0,
    Math.floor(virtualScrollTop.value / VIRTUAL_LINE_HEIGHT) -
      VIRTUAL_OVERSCAN_LINES,
  ),
);
const virtualVisibleLineCount = computed(
  () =>
    Math.ceil(virtualViewportHeight.value / VIRTUAL_LINE_HEIGHT) +
    VIRTUAL_OVERSCAN_LINES * 2,
);
const virtualEndLine = computed(() =>
  Math.min(
    virtualTotalLines.value,
    virtualStartLine.value + virtualVisibleLineCount.value,
  ),
);
const virtualTopOffset = computed(
  () => virtualStartLine.value * VIRTUAL_LINE_HEIGHT,
);
const virtualRenderedLines = computed(() =>
  streamLines.value.slice(virtualStartLine.value, virtualEndLine.value),
);
const virtualStatus = computed(() => {
  if (!isStreamingText.value) return "";
  if (!virtualTotalLines.value)
    return streamLoading.value ? "Loading first slice…" : "No text loaded";
  return `Rendering lines ${virtualStartLine.value + 1}-${virtualEndLine.value} of ${virtualTotalLines.value}`;
});
const hexCanPrev = computed(() => (hexData.value?.offset || 0) > 0);
const hexCanNext = computed(() => Boolean(hexData.value && !hexData.value.eof));

const downloadUrl = computed(() => {
  if (!props.preview?.path) return "";
  return `/system/download?path=${encodeURIComponent(props.preview.path)}`;
});
const pdfUrl = computed(() =>
  downloadUrl.value ? `${downloadUrl.value}#toolbar=1&navpanes=1` : "",
);
const videoUrl = computed(() => downloadUrl.value);

const stopStreaming = () => {
  if (streamAbortController) {
    streamAbortController.abort();
    streamAbortController = null;
  }
  streamLoading.value = false;
};

const resetStreamState = () => {
  stopStreaming();
  streamError.value = "";
  streamBytesLoaded.value = 0;
  streamLineCarry = "";
  pendingStreamLineCount = 0;
  virtualScrollTop.value = 0;
  streamLines.value = [];
};

const resetHexState = () => {
  hexLoading.value = false;
  hexError.value = "";
  hexData.value = null;
  hexOffsetInput.value = 0;
};

const resetElfState = () => {
  elfLoading.value = false;
  elfError.value = "";
  elfData.value = null;
};

const syncVirtualViewport = async () => {
  await nextTick();
  const el = virtualScrollerRef.value;
  if (!el) return;
  virtualViewportHeight.value =
    el.clientHeight || VIRTUAL_DEFAULT_VIEWPORT_HEIGHT;
};

const handleVirtualScroll = (event: Event) => {
  const el = event.currentTarget as HTMLElement;
  virtualScrollTop.value = el.scrollTop;
  virtualViewportHeight.value =
    el.clientHeight || VIRTUAL_DEFAULT_VIEWPORT_HEIGHT;
};

const flushStreamLines = (force = false) => {
  if (!force && pendingStreamLineCount < STREAM_CHUNK_FLUSH_LINES) return;
  pendingStreamLineCount = 0;
  triggerRef(streamLines);
};

const appendStreamText = (text: string) => {
  if (!text) return;
  const parts = (streamLineCarry + text.replace(/^﻿/, "")).split(/\r?\n/);
  streamLineCarry = parts.pop() ?? "";
  if (!parts.length) return;
  streamLines.value.push(...parts);
  pendingStreamLineCount += parts.length;
  flushStreamLines();
};

const finishStreamLines = () => {
  if (streamLineCarry || !streamLines.value.length) {
    streamLines.value.push(streamLineCarry.replace(/^﻿/, ""));
  }
  streamLineCarry = "";
  flushStreamLines(true);
};

const normalizeTextEncoding = (encoding?: string) => {
  const value = (encoding || "utf-8").toLowerCase();
  if (value === "utf-16le" || value === "utf-16be") return value;
  return "utf-8";
};

const streamDecoderEncoding = computed(() =>
  normalizeTextEncoding(props.preview?.encoding),
);

const streamTextPreview = async (path: string) => {
  resetStreamState();
  void syncVirtualViewport();
  const controller = new AbortController();
  streamAbortController = controller;
  streamLoading.value = true;

  try {
    const response = await fetch(
      `/system/file-preview/stream?path=${encodeURIComponent(path)}`,
      {
        headers: buildRequestHeaders(),
        signal: controller.signal,
      },
    );
    if (!response.ok) {
      throw new Error(`Stream failed: ${response.status}`);
    }
    if (!response.body) {
      throw new Error("Streaming is not supported by this browser");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder(streamDecoderEncoding.value);
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      if (!value) continue;
      streamBytesLoaded.value += value.byteLength;
      appendStreamText(decoder.decode(value, { stream: true }));
    }
    appendStreamText(decoder.decode());
    finishStreamLines();
  } catch (err) {
    if ((err as Error).name !== "AbortError") {
      streamError.value =
        (err as Error).message || "Failed to stream file preview";
    }
  } finally {
    if (streamAbortController === controller) {
      streamAbortController = null;
      streamLoading.value = false;
    }
  }
};

const loadHexPage = async (offset = 0) => {
  if (!props.preview?.path) return;
  hexLoading.value = true;
  hexError.value = "";
  try {
    const response = await fetch(
      `/system/file-hex?path=${encodeURIComponent(props.preview.path)}&offset=${Math.max(0, offset)}&limit=${HEX_PAGE_SIZE}`,
      {
        headers: buildRequestHeaders(),
      },
    );
    if (!response.ok) throw new Error(`Hex load failed: ${response.status}`);
    const payload = (await response.json()) as HexResponse;
    hexData.value = payload;
    hexOffsetInput.value = payload.offset;
  } catch (err) {
    hexError.value = (err as Error).message || "Failed to load hex view";
  } finally {
    hexLoading.value = false;
  }
};

const loadELFAnalysis = async () => {
  if (!props.preview?.path || props.preview.previewType !== "elf") return;
  elfLoading.value = true;
  elfError.value = "";
  try {
    const response = await fetch(
      `/system/file-elf?path=${encodeURIComponent(props.preview.path)}`,
      {
        headers: buildRequestHeaders(),
      },
    );
    if (!response.ok)
      throw new Error(`ELF analysis failed: ${response.status}`);
    elfData.value = (await response.json()) as ELFAnalysis;
  } catch (err) {
    elfError.value = (err as Error).message || "Failed to analyze ELF file";
  } finally {
    elfLoading.value = false;
  }
};

const showHexPanel = async () => {
  activePanel.value = "hex";
  if (!hexData.value) await loadHexPage(0);
};

const showPreviewPanel = () => {
  activePanel.value = "preview";
};

watch(
  () => props.open,
  (isOpen) => {
    if (!isOpen) {
      highlightedHtml.value = "";
      highlightLoading.value = false;
      resetStreamState();
      resetHexState();
      resetElfState();
      activePanel.value = "preview";
      return;
    }
    void syncVirtualViewport();
  },
);

watch(
  () =>
    [
      props.open,
      props.preview?.path,
      props.preview?.previewType,
      props.preview?.streamable,
    ] as const,
  ([isOpen, path, previewType, streamable]) => {
    resetHexState();
    resetElfState();
    activePanel.value = "preview";
    if (isOpen && path && previewType === "text" && streamable) {
      void streamTextPreview(path);
      return;
    }
    resetStreamState();
    if (isOpen && path && previewType === "elf") {
      void loadELFAnalysis();
    }
  },
  { immediate: true },
);

watch(
  () =>
    [
      props.open,
      props.preview?.path,
      props.preview?.previewType,
      props.preview?.language,
      props.preview?.content,
      props.preview?.streamable,
    ] as const,
  async ([isOpen, , previewType, language, content, streamable]) => {
    const runId = ++highlightRunId;
    if (isOpen && previewType === "text" && content && !streamable) {
      highlightLoading.value = true;
      highlightedHtml.value = "";
      try {
        const { highlightPreviewCode } = await loadCodeHighlightingModule();
        const html = await highlightPreviewCode(content, language);
        if (runId !== highlightRunId) return;
        highlightedHtml.value = html;
      } catch (err) {
        console.error("Failed to highlight code", err);
        if (runId === highlightRunId) highlightedHtml.value = "";
      } finally {
        if (runId === highlightRunId) highlightLoading.value = false;
      }
    } else {
      highlightedHtml.value = "";
      highlightLoading.value = false;
    }
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  resetStreamState();
});
</script>

<template>
  <a-drawer
    v-model:open="drawerOpen"
    :title="title"
    width="85vw"
    destroyOnClose
  >
    <a-spin :spinning="loading">
      <a-empty v-if="!preview && !loading" description="No preview available" />

      <template v-else-if="preview">
        <a-descriptions
          bordered
          :column="2"
          size="small"
          style="margin-bottom: 16px"
        >
          <a-descriptions-item label="Path" :span="2">
            <a-typography-text
              code
              style="word-break: break-all; color: #111827"
              >{{ preview.path }}</a-typography-text
            >
          </a-descriptions-item>
          <a-descriptions-item label="Type">
            <a-tag :color="preview.isDir ? 'blue' : 'default'">{{
              preview.previewType.toUpperCase()
            }}</a-tag>
            <a-tag v-if="preview.mimeType">{{ preview.mimeType }}</a-tag>
            <a-tag v-if="preview.encoding">{{ preview.encoding }}</a-tag>
            <a-tag v-if="preview.streamable" color="green"
              >VIRTUAL STREAM</a-tag
            >
            <a-tag v-else-if="preview.truncated" color="orange"
              >TRUNCATED</a-tag
            >
            <a-tag v-if="preview.hexable" color="purple">16HEX</a-tag>
          </a-descriptions-item>
          <a-descriptions-item label="Size">{{
            formatBytes(preview.size)
          }}</a-descriptions-item>
          <a-descriptions-item label="Mode">{{
            preview.mode || "—"
          }}</a-descriptions-item>
          <a-descriptions-item label="Modified">{{
            formattedModTime
          }}</a-descriptions-item>
        </a-descriptions>

        <div v-if="canShowHex" class="file-preview-drawer__mode-toolbar">
          <a-radio-group v-model:value="activePanel" size="small">
            <a-radio-button value="preview" @click="showPreviewPanel"
              >Preview</a-radio-button
            >
            <a-radio-button value="hex" @click="showHexPanel"
              >16 Hex</a-radio-button
            >
          </a-radio-group>
        </div>

        <div v-if="isHexPanel" class="file-preview-drawer__content">
          <div class="file-preview-drawer__hex-toolbar">
            <a-button
              size="small"
              :disabled="!hexCanPrev || hexLoading"
              @click="
                loadHexPage(Math.max(0, (hexData?.offset || 0) - HEX_PAGE_SIZE))
              "
              >Prev</a-button
            >
            <a-input-number
              v-model:value="hexOffsetInput"
              :min="0"
              :max="Math.max(0, preview.size - 1)"
              size="small"
              class="file-preview-drawer__hex-offset"
            />
            <a-button
              size="small"
              :loading="hexLoading"
              @click="loadHexPage(hexOffsetInput)"
              >Go</a-button
            >
            <a-button
              size="small"
              :disabled="!hexCanNext || hexLoading"
              @click="loadHexPage(hexData?.nextOffset || 0)"
              >Next</a-button
            >
            <span v-if="hexData" class="file-preview-drawer__muted"
              >{{ formatHexOffset(hexData.offset) }} ·
              {{ hexData.bytesRead }} bytes</span
            >
          </div>
          <a-alert
            v-if="hexError"
            type="error"
            show-icon
            :message="hexError"
            style="margin-bottom: 12px"
          />
          <a-spin :spinning="hexLoading">
            <div class="file-preview-drawer__hex-table">
              <div
                v-for="row in hexData?.rows || []"
                :key="row.offset"
                class="file-preview-drawer__hex-row"
              >
                <span class="file-preview-drawer__hex-address">{{
                  formatHexOffset(row.offset)
                }}</span>
                <span class="file-preview-drawer__hex-bytes">
                  <span
                    v-for="(cell, index) in row.hex"
                    :key="`${row.offset}-${index}`"
                    >{{ cell }}</span
                  >
                </span>
                <span class="file-preview-drawer__hex-ascii">{{
                  row.ascii
                }}</span>
              </div>
            </div>
          </a-spin>
        </div>

        <template v-else>
          <a-alert
            v-if="preview.previewType === 'directory'"
            type="info"
            show-icon
            message="Directory selected"
            description="Directories can be jumped to, but not inline-previewed as file content."
          />

          <div
            v-else-if="preview.previewType === 'image'"
            class="file-preview-drawer__content"
          >
            <a-alert
              v-if="!preview.dataUrl && preview.content"
              type="info"
              show-icon
              :message="preview.content"
            />
            <img
              v-else-if="preview.dataUrl"
              :src="preview.dataUrl"
              :alt="preview.name"
              class="file-preview-drawer__image"
            />
          </div>

          <div
            v-else-if="preview.previewType === 'pdf'"
            class="file-preview-drawer__content"
          >
            <div class="file-preview-drawer__pdf-toolbar">
              <a-button
                size="small"
                :href="downloadUrl"
                target="_blank"
                rel="noopener noreferrer"
                >Open in new tab</a-button
              >
              <a-button size="small" :href="downloadUrl" download
                >Download</a-button
              >
            </div>
            <iframe
              class="file-preview-drawer__pdf"
              :src="pdfUrl"
              :title="preview.name"
            />
          </div>

          <div
            v-else-if="preview.previewType === 'video'"
            class="file-preview-drawer__content"
          >
            <video
              v-if="preview.mimeType.startsWith('video/')"
              controls
              autoplay
              class="file-preview-drawer__video"
              :src="videoUrl"
            >
              Your browser does not support the video tag.
            </video>
            <div
              v-else-if="preview.mimeType.startsWith('audio/')"
              class="file-preview-drawer__audio"
            >
              <div class="file-preview-drawer__audio-icon">🎵</div>
              <audio
                controls
                autoplay
                class="file-preview-drawer__audio-player"
                :src="videoUrl"
              >
                Your browser does not support the audio element.
              </audio>
              <div class="file-preview-drawer__audio-name">
                {{ preview.name }}
              </div>
            </div>
          </div>

          <div
            v-else-if="preview.previewType === 'elf'"
            class="file-preview-drawer__content"
          >
            <a-alert
              type="info"
              show-icon
              message="ELF / shared object analysis"
              description="Read-only structural summary with limited objdump disassembly preview when objdump is available."
              style="margin-bottom: 12px"
            />
            <a-alert
              v-if="elfError"
              type="error"
              show-icon
              :message="elfError"
              style="margin-bottom: 12px"
            />
            <a-spin :spinning="elfLoading">
              <template v-if="elfData">
                <a-descriptions
                  bordered
                  size="small"
                  :column="3"
                  style="margin-bottom: 12px"
                >
                  <a-descriptions-item
                    v-for="[key, value] in objectEntries(elfData.header)"
                    :key="key"
                    :label="key"
                    >{{ value }}</a-descriptions-item
                  >
                </a-descriptions>
                <div class="file-preview-drawer__elf-grid">
                  <div class="file-preview-drawer__elf-card">
                    <h4>Dynamic Libraries</h4>
                    <a-tag v-for="lib in elfData.dynamicLibraries" :key="lib">{{
                      lib
                    }}</a-tag>
                    <a-empty
                      v-if="!elfData.dynamicLibraries?.length"
                      description="No DT_NEEDED entries"
                    />
                  </div>
                  <div class="file-preview-drawer__elf-card">
                    <h4>Symbols</h4>
                    <p>
                      Dynamic: {{ elfData.dynamicSymbolCount }} · Static:
                      {{ elfData.staticSymbolCount }}
                    </p>
                  </div>
                </div>
                <h4>Program Headers</h4>
                <div class="file-preview-drawer__table-scroll">
                  <table class="file-preview-drawer__mini-table">
                    <tbody>
                      <tr
                        v-for="(program, index) in elfData.programs"
                        :key="index"
                      >
                        <td
                          v-for="[key, value] in objectEntries(program)"
                          :key="key"
                        >
                          <strong>{{ key }}</strong
                          >: {{ value }}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <h4>Sections</h4>
                <div class="file-preview-drawer__table-scroll">
                  <table class="file-preview-drawer__mini-table">
                    <tbody>
                      <tr
                        v-for="section in elfData.sections"
                        :key="`${section.name}-${section.offset}`"
                      >
                        <td>{{ section.name }}</td>
                        <td>{{ section.type }}</td>
                        <td>{{ section.flags }}</td>
                        <td>{{ section.offset }}</td>
                        <td>{{ section.size }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <h4>Disassembly preview</h4>
                <a-alert
                  v-if="elfData.disassemblyError"
                  type="warning"
                  show-icon
                  :message="elfData.disassemblyError"
                  style="margin-bottom: 8px"
                />
                <pre class="file-preview-drawer__pre">{{
                  elfData.disassembly || "No disassembly available."
                }}</pre>
              </template>
            </a-spin>
          </div>

          <div
            v-else-if="preview.previewType === 'text'"
            class="file-preview-drawer__content"
          >
            <div class="file-preview-drawer__text-toolbar">
              <span>Language: {{ preview.language || "text" }}</span>
              <template v-if="preview.streamable">
                <span
                  >{{ formatBytes(streamBytesLoaded) }} /
                  {{ formatBytes(preview.size) }}</span
                >
                <span>{{ virtualStatus }}</span>
                <a-progress
                  :percent="streamProgressPercent"
                  size="small"
                  class="file-preview-drawer__progress"
                />
                <a-button
                  v-if="streamLoading"
                  size="small"
                  @click="stopStreaming"
                  >Stop</a-button
                >
              </template>
              <a-checkbox
                v-if="!isStreamingText"
                v-model:checked="wordWrap"
                size="small"
                >Word Wrap</a-checkbox
              >
            </div>
            <a-alert
              v-if="isStreamingText"
              type="info"
              show-icon
              message="Virtualized stream preview"
              description="The file is sliced into lines and only the visible window is rendered into DOM. Word wrap is disabled for stable virtual scrolling."
              style="margin-bottom: 12px"
            />
            <a-alert
              v-if="streamError"
              type="error"
              show-icon
              :message="streamError"
              style="margin-bottom: 12px"
            />
            <div
              v-if="isStreamingText"
              ref="virtualScrollerRef"
              class="file-preview-drawer__virtual-scroller"
              @scroll="handleVirtualScroll"
            >
              <div
                class="file-preview-drawer__virtual-spacer"
                :style="{ height: `${virtualTotalHeight}px` }"
              >
                <pre
                  class="file-preview-drawer__virtual-pre"
                  :style="{ transform: `translateY(${virtualTopOffset}px)` }"
                ><template v-for="(line, index) in virtualRenderedLines" :key="virtualStartLine + index">{{ line }}{{ '\n' }}</template></pre>
              </div>
            </div>
            <a-spin v-else :spinning="highlightLoading">
              <div
                v-if="highlightedHtml"
                class="file-preview-drawer__shiki"
                :class="{ 'is-wrapped': wordWrap }"
                v-html="highlightedHtml"
              />
              <pre
                v-else
                class="file-preview-drawer__pre"
                :class="{ 'is-wrapped': wordWrap }"
                >{{ preview.content }}</pre
              >
            </a-spin>
          </div>

          <div v-else class="file-preview-drawer__content">
            <a-alert
              type="warning"
              show-icon
              message="Binary file preview"
              description="Showing a limited hex dump. Use the 16 Hex view for paged binary inspection."
              style="margin-bottom: 12px"
            />
            <pre class="file-preview-drawer__pre">{{
              preview.content || "Binary preview unavailable."
            }}</pre>
          </div>
        </template>
      </template>
    </a-spin>
  </a-drawer>
</template>

<style scoped>
.file-preview-drawer__content {
  max-width: 100%;
}

.file-preview-drawer__mode-toolbar,
.file-preview-drawer__text-toolbar,
.file-preview-drawer__hex-toolbar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 8px;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
  font-size: 12px;
  color: #4b5563;
}

.file-preview-drawer__progress {
  width: 180px;
}

.file-preview-drawer__muted {
  color: #64748b;
}

.file-preview-drawer__hex-offset {
  width: 160px;
}

.file-preview-drawer__hex-table,
.file-preview-drawer__pre,
.file-preview-drawer__shiki :deep(pre) {
  margin: 0;
  padding: 16px;
  max-height: calc(100vh - 280px);
  overflow: auto;
  border-radius: 8px;
  background: #0f172a !important;
  color: #e2e8f0;
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
}

.file-preview-drawer__hex-row {
  display: grid;
  grid-template-columns: 110px minmax(420px, max-content) 180px;
  gap: 16px;
  white-space: pre;
}

.file-preview-drawer__hex-address {
  color: #93c5fd;
}

.file-preview-drawer__hex-bytes {
  display: grid;
  grid-template-columns: repeat(16, 24px);
  color: #e2e8f0;
}

.file-preview-drawer__hex-ascii {
  color: #bbf7d0;
}

.file-preview-drawer__pdf-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 12px;
}

.file-preview-drawer__pdf {
  display: block;
  width: 100%;
  height: calc(100vh - 320px);
  min-height: 520px;
  border: 1px solid #d9d9d9;
  border-radius: 8px;
  background: #f8fafc;
}

.file-preview-drawer__video {
  width: 100%;
  max-height: 70vh;
  border-radius: 8px;
  background: #000;
}

.file-preview-drawer__audio {
  padding: 40px;
  background: #f0f2f5;
  border-radius: 8px;
  text-align: center;
}

.file-preview-drawer__audio-icon {
  margin-bottom: 16px;
  font-size: 48px;
}

.file-preview-drawer__audio-player {
  width: 100%;
}

.file-preview-drawer__audio-name {
  margin-top: 12px;
  color: #4a4a4a;
  font-size: 14px;
}

.file-preview-drawer__elf-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.file-preview-drawer__elf-card {
  padding: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #f8fafc;
}

.file-preview-drawer__table-scroll {
  max-height: 240px;
  overflow: auto;
  margin-bottom: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
}

.file-preview-drawer__mini-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.file-preview-drawer__mini-table td {
  padding: 6px 8px;
  border-bottom: 1px solid #eef2f7;
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
}

.file-preview-drawer__virtual-scroller {
  height: calc(100vh - 330px);
  min-height: 360px;
  overflow: auto;
  border-radius: 8px;
  background: #0f172a;
}

.file-preview-drawer__virtual-spacer {
  position: relative;
  min-width: max-content;
}

.file-preview-drawer__virtual-pre {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  margin: 0;
  padding: 16px;
  color: #e2e8f0;
  font-family: "JetBrains Mono", "SFMono-Regular", Consolas, monospace;
  font-size: 13px;
  line-height: 21px;
  white-space: pre;
  overflow: visible;
}

.file-preview-drawer__shiki :deep(.line) {
  display: block;
  min-height: 1.5em;
}

.is-wrapped,
.is-wrapped :deep(pre) {
  white-space: pre-wrap !important;
  word-break: break-all !important;
  overflow-wrap: anywhere !important;
}

.file-preview-drawer__image {
  display: block;
  max-width: 100%;
  max-height: calc(100vh - 260px);
  margin: 0 auto;
  border-radius: 8px;
  border: 1px solid #f0f0f0;
  background: #fafafa;
}
</style>
