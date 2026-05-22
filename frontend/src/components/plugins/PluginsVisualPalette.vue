<script setup lang="ts">
import { DragOutlined } from "@ant-design/icons-vue";
import { triggerOptions, fieldOptions } from "./constants";

const handleDragStart = (event: DragEvent, category: string, value: string) => {
  if (event.dataTransfer) {
    event.dataTransfer.setData("text/plain", JSON.stringify({ category, value }));
    event.dataTransfer.effectAllowed = "move";
  }
};
</script>

<template>
  <div class="blueprint-palette">
    <div class="palette-header">
      <DragOutlined class="palette-icon" />
      <h4>蓝图组件库 (Palette)</h4>
    </div>
    <div class="palette-desc">
      拖拽下列组件到右侧画布即可快速拼接 eBPF 过滤流。
    </div>
    
    <!-- Category 1: Trigger Hooks -->
    <div class="palette-category">
      <div class="category-title">事件触发器 (Triggers)</div>
      <div class="palette-items">
        <div
          v-for="opt in triggerOptions"
          :key="opt.value"
          class="palette-item item-trigger"
          draggable="true"
          @dragstart="handleDragStart($event, 'trigger', opt.value)"
        >
          <component :is="opt.icon" :style="{ color: '#1890ff', marginRight: '6px' }" />
          <span class="item-text" :title="opt.label">{{ opt.value }}</span>
        </div>
      </div>
    </div>

    <!-- Category 2: Conditions -->
    <div class="palette-category">
      <div class="category-title">过滤条件 (Conditions)</div>
      <div class="palette-items">
        <div
          v-for="opt in fieldOptions"
          :key="opt.value"
          class="palette-item item-condition"
          draggable="true"
          @dragstart="handleDragStart($event, 'condition', opt.value)"
        >
          <span class="item-dot condition-dot"></span>
          <span class="item-text" :title="opt.label">{{ opt.value }}</span>
        </div>
      </div>
    </div>

    <!-- Category 2.5: Logic Groups -->
    <div class="palette-category">
      <div class="category-title">逻辑分组 (Logic Groups)</div>
      <div class="palette-items">
        <div
          class="palette-item item-group-and"
          style="border-left: 3px solid #1890ff;"
          draggable="true"
          @dragstart="handleDragStart($event, 'logic_group', 'AND')"
        >
          <span class="item-dot" style="background: #1890ff; box-shadow: 0 0 6px #1890ff;"></span>
          <span class="item-text" title="且运算组 (AND Group)">AND Group</span>
        </div>
        <div
          class="palette-item item-group-or"
          style="border-left: 3px solid #eb2f96;"
          draggable="true"
          @dragstart="handleDragStart($event, 'logic_group', 'OR')"
        >
          <span class="item-dot" style="background: #eb2f96; box-shadow: 0 0 6px #eb2f96;"></span>
          <span class="item-text" title="或运算组 (OR Group)">OR Group</span>
        </div>
      </div>
    </div>

    <!-- Category 3: Map Operations -->
    <div class="palette-category">
      <div class="category-title">状态机制 (State Maps)</div>
      <div class="palette-items">
        <div
          class="palette-item item-map"
          draggable="true"
          @dragstart="handleDragStart($event, 'map', 'COUNTER')"
        >
          <span class="item-dot map-dot"></span>
          <span class="item-text" title="计数器限频 (COUNTER)">COUNTER</span>
        </div>
        <div
          class="palette-item item-map"
          draggable="true"
          @dragstart="handleDragStart($event, 'map', 'BLOCKLIST')"
        >
          <span class="item-dot map-dot"></span>
          <span class="item-text" title="黑名单判定 (BLOCKLIST)">BLOCKLIST</span>
        </div>
        <div
          class="palette-item item-map"
          draggable="true"
          @dragstart="handleDragStart($event, 'map', 'NONE')"
        >
          <span class="item-dot map-dot-none"></span>
          <span class="item-text" title="无状态 (NONE)">NONE</span>
        </div>
      </div>
    </div>

    <!-- Category 4: Response Actions -->
    <div class="palette-category">
      <div class="category-title">响应动作 (Actions)</div>
      <div class="palette-items">
        <div
          class="palette-item item-action"
          draggable="true"
          @dragstart="handleDragStart($event, 'action', 'BLOCK')"
        >
          <span class="item-dot action-dot"></span>
          <span class="item-text" title="硬拦截 (BLOCK)">BLOCK</span>
        </div>
        <div
          class="palette-item item-action"
          draggable="true"
          @dragstart="handleDragStart($event, 'action', 'ALERT')"
        >
          <span class="item-dot action-dot"></span>
          <span class="item-text" title="告警审计 (ALERT)">ALERT</span>
        </div>
        <div
          class="palette-item item-action"
          draggable="true"
          @dragstart="handleDragStart($event, 'action', 'KILL')"
        >
          <span class="item-dot action-dot"></span>
          <span class="item-text" title="强制杀死 (KILL)">KILL</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* UE Blueprint Palette Styling */
.blueprint-palette {
  background-color: #121620;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 8px;
  padding: 16px;
  min-height: 550px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  font-family: monospace;
}

.palette-header {
  display: flex;
  align-items: center;
  margin-bottom: 8px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  padding-bottom: 8px;
}

.palette-icon {
  font-size: 14px;
  margin-right: 6px;
  color: #fa8c16;
}

.palette-header h4 {
  margin: 0;
  color: #f1f5f9;
  font-size: 13px;
  font-weight: 600;
}

.palette-desc {
  font-size: 10px;
  color: #64748b;
  line-height: 1.4;
  margin-bottom: 16px;
}

.palette-category {
  margin-bottom: 16px;
}

.category-title {
  font-size: 11px;
  font-weight: bold;
  color: #94a3b8;
  margin-bottom: 8px;
  padding-bottom: 2px;
  border-bottom: 1px dashed rgba(255, 255, 255, 0.05);
  text-transform: uppercase;
}

.palette-items {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.palette-item {
  background: rgba(30, 41, 59, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.03);
  border-radius: 4px;
  padding: 6px 8px;
  font-size: 11px;
  color: #cbd5e1;
  cursor: grab;
  display: flex;
  align-items: center;
  transition: all 0.2s ease;
  user-select: none;
}

.palette-item:active {
  cursor: grabbing;
}

.palette-item:hover {
  background: rgba(30, 41, 59, 0.95);
  transform: translateX(2px);
  color: #ffffff;
}

/* Color Coding for Palette Items (matching blueprint color accents) */
.item-trigger:hover {
  border-color: #1890ff;
  box-shadow: 0 0 8px rgba(24, 144, 255, 0.25);
}

.item-condition:hover {
  border-color: #fa8c16;
  box-shadow: 0 0 8px rgba(250, 140, 22, 0.25);
}

.item-map:hover {
  border-color: #722ed1;
  box-shadow: 0 0 8px rgba(114, 46, 209, 0.25);
}

.item-action:hover {
  border-color: #52c41a;
  box-shadow: 0 0 8px rgba(82, 196, 26, 0.25);
}

.item-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Dots and accents */
.item-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  margin-right: 8px;
  flex-shrink: 0;
}

.condition-dot {
  background: #fa8c16;
  box-shadow: 0 0 6px #fa8c16;
}

.map-dot {
  background: #722ed1;
  box-shadow: 0 0 6px #722ed1;
}

.map-dot-none {
  background: #94a3b8;
}

.action-dot {
  background: #52c41a;
  box-shadow: 0 0 6px #52c41a;
}
</style>
