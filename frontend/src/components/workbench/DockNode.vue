<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from "vue";
import {
  CloseOutlined,
  ColumnWidthOutlined,
  ColumnHeightOutlined,
} from "@ant-design/icons-vue";
import PaneRoute from "./PaneRoute.vue";
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
  if (props.node.kind !== "split") return;
  event.preventDefault();
  cleanup();
  const rect = container.value!.getBoundingClientRect(),
    axis = props.node.axis;
  const move = (e: PointerEvent) =>
    emit(
      "resize",
      props.node.id,
      Math.max(
        0.15,
        Math.min(
          0.85,
          axis === "horizontal"
            ? (e.clientX - rect.left) / rect.width
            : (e.clientY - rect.top) / rect.height,
        ),
      ),
    );
  cleanup = () => {
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", cleanup);
    window.removeEventListener("pointercancel", cleanup);
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  };
  document.body.style.cursor =
    axis === "horizontal" ? "col-resize" : "row-resize";
  document.body.style.userSelect = "none";
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", cleanup);
  window.addEventListener("pointercancel", cleanup);
}
onBeforeUnmount(() => cleanup());
function resizeKey(event: KeyboardEvent) {
  if (
    props.node.kind !== "split" ||
    !["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(event.key)
  )
    return;
  event.preventDefault();
  emit(
    "resize",
    props.node.id,
    Math.max(
      0.15,
      Math.min(
        0.85,
        props.node.ratio +
          (["ArrowLeft", "ArrowUp"].includes(event.key) ? -0.05 : 0.05),
      ),
    ),
  );
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
      <div class="dock-tab-list" role="tablist" aria-label="工作台面板">
        <div
          v-for="tab in node.tabs"
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
            :aria-selected="node.active === tab.id"
            :title="tab.path + ' · 拖拽到边缘分屏'"
            @click="emit('select', node.id, tab.id)"
          >
            <span class="tab-mark">◇</span>{{ tab.title }}
          </button>
          <button
            class="tab-close"
            :aria-label="'关闭 ' + tab.title"
            @click="emit('close', node.id, tab.id)"
          >
            <CloseOutlined />
          </button>
        </div>
      </div>
      <div class="dock-actions">
        <button
          title="向右分屏"
          aria-label="向右分屏"
          @click="emit('split', node.id, 'horizontal')"
        >
          <ColumnWidthOutlined /></button
        ><button
          title="向下分屏"
          aria-label="向下分屏"
          @click="emit('split', node.id, 'vertical')"
        >
          <ColumnHeightOutlined />
        </button>
      </div>
    </header>
    <div class="dock-breadcrumb">
      工作空间 <span>›</span> {{ activeTab?.title || "欢迎" }}
      <span class="dock-breadcrumb-path">{{ activeTab?.path }}</span>
    </div>
    <div class="dock-content" role="tabpanel">
      <template v-for="tab in node.tabs" :key="tab.id">
        <PaneRoute
          v-if="tab.id === node.active"
          :path="tab.path"
          @navigate="(path) => emit('navigate', node.id, tab.id, path)"
        />
      </template>
      <div v-if="!node.tabs.length" class="dock-empty">
        <div>⌘</div>
        <h2>Agent 工作空间</h2>
        <p>从左侧打开功能，或将标签拖到这里</p>
        <p>拖到边缘分屏 · 拖到中央合并 · 拖动分隔线调整大小</p>
      </div>
    </div>
    <div v-if="dragging && hover" class="dock-drop-preview" :class="hover">
      <span>{{ hover === "center" ? "合并到此面板" : "释放以创建分屏" }}</span>
    </div>
  </section>
</template>
