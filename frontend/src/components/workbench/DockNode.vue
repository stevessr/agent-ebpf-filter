<script setup lang="ts">
import { computed, ref, onBeforeUnmount, nextTick } from "vue";
import { dockTabNavigationIndex, dockSplitterRatio, dockPointerRatio } from "../../composables/workbench/keyboardNavigation";
import {
  CloseOutlined,
  ColumnWidthOutlined,
  ColumnHeightOutlined,
} from "@ant-design/icons-vue";
import PaneRoute from "./PaneRoute.vue";
import { translateUI as tr } from "../../i18n";
import type { DockNode, DockEdge } from "../../composables/workbench/layout";
const props = defineProps<{
  node: DockNode;
  focused: string;
  dragging: boolean;
}>();
const emit = defineEmits<{
  focus: [id: string];
  select: [group: string, tab: string];
  close: [group: string, tab: string];
  drag: [tab: string, event: DragEvent];
  end: [];
  drop: [group: string, edge: DockEdge, event: DragEvent];
  split: [group: string, axis: "horizontal" | "vertical"];
  resize: [id: string, ratio: number];
  navigate: [group: string, tab: string, path: string];
}>();
const container = ref<HTMLElement>();
const hover = ref<DockEdge>();
const activeTab = computed(() =>
  props.node.kind === "group"
    ? props.node.tabs.find(
        (t) => t.id === (props.node.kind === "group" ? props.node.active : ""),
      )
    : undefined,
);
function over(event: DragEvent) {
  if (!props.dragging) return;
  event.preventDefault();
  const rect = container.value!.getBoundingClientRect();
  const x = (event.clientX - rect.left) / rect.width,
    y = (event.clientY - rect.top) / rect.height;
  hover.value =
    x < 0.22
      ? "left"
      : x > 0.78
        ? "right"
        : y < 0.22
          ? "top"
          : y > 0.78
            ? "bottom"
            : "center";
}
function drop(event: DragEvent) {
  if (!props.dragging) return;
  event.preventDefault();
  emit("drop", props.node.id, hover.value || "center", event);
  hover.value = undefined;
}
let cleanup = () => {};
function startResize(event: PointerEvent) {
  if (props.node.kind !== "split" || !event.isPrimary || event.button !== 0 || !container.value) return;
  const rect = container.value.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return;
  event.preventDefault();
  cleanup();
  const axis = props.node.axis;
  const pointerId = event.pointerId;
  const previousCursor = document.body.style.cursor;
  const previousSelect = document.body.style.userSelect;
  let frame: number | undefined;
  let pendingRatio: number | undefined;
  let lastCommitted = props.node.ratio;
  function flush() {
    if (frame !== undefined) {
      cancelAnimationFrame(frame);
      frame = undefined;
    }
    if (pendingRatio === undefined) return;
    const ratio = pendingRatio;
    pendingRatio = undefined;
    if (ratio === lastCommitted) return;
    lastCommitted = ratio;
    emit("resize", props.node.id, ratio);
  }
  function queue(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    const ratio = dockPointerRatio(axis, e.clientX, e.clientY, rect);
    if (ratio === null) return;
    pendingRatio = ratio;
    if (frame === undefined) frame = requestAnimationFrame(flush);
  }
  function end(e: PointerEvent) {
    if (e.pointerId !== pointerId) return;
    queue(e);
    flush();
    cleanup();
  }
  function cancel(e: PointerEvent) {
    if (e.pointerId === pointerId) cleanup();
  }
  cleanup = () => {
    window.removeEventListener("pointermove", queue);
    window.removeEventListener("pointerup", end);
    window.removeEventListener("pointercancel", cancel);
    window.removeEventListener("blur", cleanup);
    if (frame !== undefined) cancelAnimationFrame(frame);
    frame = undefined;
    pendingRatio = undefined;
    document.body.style.cursor = previousCursor;
    document.body.style.userSelect = previousSelect;
  };
  document.body.style.cursor = axis === "horizontal" ? "col-resize" : "row-resize";
  document.body.style.userSelect = "none";
  window.addEventListener("pointermove", queue);
  window.addEventListener("pointerup", end);
  window.addEventListener("pointercancel", cancel);
  window.addEventListener("blur", cleanup);
}
onBeforeUnmount(() => cleanup());
function resizeKey(event: KeyboardEvent) {
  if (props.node.kind !== "split" || event.altKey || event.ctrlKey || event.metaKey) return;
  const ratio = dockSplitterRatio(props.node.axis, event.key, props.node.ratio);
  if (ratio === null) return;
  event.preventDefault();
  emit("resize", props.node.id, ratio);
}
function navigateTab(event: KeyboardEvent, index: number) {
  if (props.node.kind !== "group" || event.altKey || event.ctrlKey || event.metaKey) return;
  const next = dockTabNavigationIndex(event.key, index, props.node.tabs.length);
  if (next === null) return;
  event.preventDefault();
  const tab = props.node.tabs[next];
  if (!tab) return;
  const list = (event.currentTarget as HTMLElement | null)?.closest(".dock-tab-list");
  emit("select", props.node.id, tab.id);
  void nextTick(() => list?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus());
}
</script>
<template>
  <div
    v-if="node.kind === 'split'"
    ref="container"
    class="dock-split"
    :class="node.axis"
  >
    <div class="dock-branch" :style="{ flex: `${node.ratio} 1 0%` }">
      <DockNode
        :node="node.first"
        :focused="focused"
        :dragging="dragging"
        @focus="emit('focus', $event)"
        @select="(g, t) => emit('select', g, t)"
        @close="(g, t) => emit('close', g, t)"
        @drag="(t, e) => emit('drag', t, e)"
        @end="emit('end')"
        @drop="(g, s, e) => emit('drop', g, s, e)"
        @split="(g, a) => emit('split', g, a)"
        @resize="(id, r) => emit('resize', id, r)"
        @navigate="(g, t, p) => emit('navigate', g, t, p)"
      />
    </div>
    <div
      class="dock-resizer"
      role="separator"
      tabindex="0"
      aria-label="调整分屏大小"
      :aria-orientation="node.axis === 'horizontal' ? 'vertical' : 'horizontal'"
      :aria-valuenow="Math.round(node.ratio * 100)"
      :aria-valuemin="15"
      :aria-valuemax="85"
      @pointerdown="startResize"
      @keydown="resizeKey"
    />
    <div class="dock-branch" :style="{ flex: `${1 - node.ratio} 1 0%` }">
      <DockNode
        :node="node.second"
        :focused="focused"
        :dragging="dragging"
        @focus="emit('focus', $event)"
        @select="(g, t) => emit('select', g, t)"
        @close="(g, t) => emit('close', g, t)"
        @drag="(t, e) => emit('drag', t, e)"
        @end="emit('end')"
        @drop="(g, s, e) => emit('drop', g, s, e)"
        @split="(g, a) => emit('split', g, a)"
        @resize="(id, r) => emit('resize', id, r)"
        @navigate="(g, t, p) => emit('navigate', g, t, p)"
      />
    </div>
  </div>
  <section
    v-else
    ref="container"
    class="dock-group"
    :class="{ focused: focused === node.id }"
    @pointerdown="emit('focus', node.id)"
    @focusin="emit('focus', node.id)"
    @dragover="over"
    @dragleave="
      (event) => {
        if (!container?.contains(event.relatedTarget as Node))
          hover = undefined;
      }
    "
    @drop="drop"
  >
    <header
      class="dock-tabs"
      @dragover.stop="
        (event) => {
          if (dragging) {
            event.preventDefault();
            hover = 'center';
          }
        }
      "
      @drop.stop="drop"
    >
      <div class="dock-tab-list" role="tablist" aria-label="工作台面板" aria-orientation="horizontal">
        <div
          v-for="(tab, index) in node.tabs"
          :key="tab.id"
          class="dock-tab"
          :class="{ active: node.active === tab.id }"
          draggable="true"
          @dragstart="emit('drag', tab.id, $event)"
          @dragend="
            hover = undefined;
            emit('end');
          "
        >
          <button
            role="tab"
            :id="`dock-tab-${node.id}-${tab.id}`"
            :aria-controls="`dock-panel-${node.id}`"
            :tabindex="node.active === tab.id ? 0 : -1"
            :aria-selected="node.active === tab.id"
            :title="tab.path + ' · ' + tr('拖拽到边缘分屏')"
            @click="emit('select', node.id, tab.id)"
            @keydown="navigateTab($event, index)"
          >
            {{ tr(tab.title) }}
          </button>
          <button
            class="tab-close"
            :aria-label="tr('关闭') + ' ' + tr(tab.title)"
            @click="emit('close', node.id, tab.id)"
          >
            <CloseOutlined aria-hidden="true" />
          </button>
        </div>
      </div>
      <div class="dock-actions">
        <button
          title="向右分屏"
          aria-label="向右分屏"
          @click="emit('split', node.id, 'horizontal')"
        >
          <ColumnWidthOutlined aria-hidden="true" /></button
        ><button
          title="向下分屏"
          aria-label="向下分屏"
          @click="emit('split', node.id, 'vertical')"
        >
          <ColumnHeightOutlined aria-hidden="true" />
        </button>
      </div>
    </header>
    <div class="dock-breadcrumb">
      {{ tr("工作空间") }} <span>›</span> {{ tr(activeTab?.title || "欢迎") }}
      <span class="dock-breadcrumb-path">{{ activeTab?.path }}</span>
    </div>
    <div class="dock-content" role="tabpanel" :id="`dock-panel-${node.id}`" :aria-labelledby="node.active ? `dock-tab-${node.id}-${node.active}` : undefined" tabindex="0">
      <template v-for="tab in node.tabs" :key="tab.id">
        <PaneRoute
          v-if="tab.id === node.active"
          :path="tab.path"
          @navigate="(path) => emit('navigate', node.id, tab.id, path)"
        />
      </template>
      <div v-if="!node.tabs.length" class="dock-empty">
        <h2>{{ tr("Agent 工作空间") }}</h2>
        <p>{{ tr("从左侧打开功能，或将标签拖到这里") }}</p>
        <p>{{ tr("拖到边缘分屏 · 拖到中央合并 · 拖动分隔线调整大小") }}</p>
      </div>
    </div>
    <div v-if="dragging && hover" class="dock-drop-preview" :class="hover">
      <span>{{ tr(hover === "center" ? "合并到此面板" : "释放以创建分屏") }}</span>
    </div>
  </section>
</template>
