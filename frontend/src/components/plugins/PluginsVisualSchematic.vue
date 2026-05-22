<script setup lang="ts">
import { computed } from "vue";
import type { VisualLogicNode, VisualLogicGroup, VisualCondition } from "./types";

interface Props {
  logicRoot: VisualLogicGroup;
}

const props = defineProps<Props>();

// Dynamic tree layout algorithm for the logic gate schematic
const logicGateLayout = computed(() => {
  const elements: Array<{
    id: string;
    type: "condition" | "gate";
    label: string;
    x: number;
    y: number;
    field?: string;
    op?: string;
    value?: string;
  }> = [];

  const wires: Array<{
    d: string;
    color: string;
  }> = [];

  const leaves: VisualCondition[] = [];
  const findLeaves = (node: VisualLogicNode) => {
    if (node.type === "CONDITION") {
      leaves.push(node as VisualCondition);
    } else if (node.children) {
      node.children.forEach(findLeaves);
    }
  };
  findLeaves(props.logicRoot);

  const numLeaves = leaves.length;
  const leafMap = new Map<string, { x: number; y: number }>();

  leaves.forEach((leaf, idx) => {
    const x = 8;
    const y = numLeaves <= 1 ? 90 : 18 + (idx * (180 - 36)) / (numLeaves - 1);
    leafMap.set(leaf.id, { x, y });

    const opLabel =
      leaf.operator === "=="
        ? "="
        : leaf.operator === "!="
        ? "≠"
        : leaf.operator === "starts_with"
        ? "pref"
        : "suff";
    elements.push({
      id: leaf.id,
      type: "condition",
      label: leaf.field,
      x,
      y,
      field: leaf.field,
      op: opLabel,
      value: leaf.value || "?",
    });
  });

  const nodePositionMap = new Map<string, { x: number; y: number }>();

  const positionNode = (node: VisualLogicNode, depth: number): { x: number; y: number } => {
    if (node.type === "CONDITION") {
      return leafMap.get(node.id) || { x: 8, y: 90 };
    }

    const childPosList: { x: number; y: number }[] = [];
    if (node.children && node.children.length > 0) {
      node.children.forEach((child) => {
        childPosList.push(positionNode(child, depth + 1));
      });
    }

    let y = 90;
    if (childPosList.length > 0) {
      y = childPosList.reduce((sum, p) => sum + p.y, 0) / childPosList.length;
    }

    let x = 180;
    if (node.id !== "root") {
      x = Math.max(50, 180 - depth * 35);
    }

    nodePositionMap.set(node.id, { x, y });

    elements.push({
      id: node.id,
      type: "gate",
      label: node.type,
      x,
      y,
    });

    if (node.children && node.children.length > 0) {
      node.children.forEach((child) => {
        const childPos =
          nodePositionMap.get(child.id) || leafMap.get(child.id) || { x: 8, y: 90 };
        const startX = childPos.x + (child.type === "CONDITION" ? 85 : 14);
        const startY = childPos.y;
        const endX = x - 14;
        const endY = y;

        const path = `M ${startX} ${startY} C ${startX + 15} ${startY}, ${endX - 15} ${endY}, ${endX} ${endY}`;
        const color =
          node.type === "AND" ? "url(#wire-gradient-and)" : "url(#wire-gradient-or)";
        wires.push({ d: path, color });
      });
    }

    return { x, y };
  };

  positionNode(props.logicRoot, 0);

  return { elements, wires };
});
</script>

<template>
  <div style="font-size: 12px; font-weight: 600; color: #fa8c16; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.5px; display: flex; align-items: center; justify-content: space-between;">
    <span>逻辑拓扑树 (Logic Tree Gate)</span>
    <a-tag size="small" color="orange" style="font-size: 10px; margin: 0; transform: scale(0.9);">Schematic</a-tag>
  </div>
  
  <div class="logic-gate-canvas">
    <div class="logic-gate-grid"></div>

    <!-- SVG containing both dynamic bezier wires and gate/node elements -->
    <svg class="logic-gate-wires" viewBox="0 0 200 180" width="100%" height="100%" preserveAspectRatio="xMidYMid meet">
      <defs>
        <linearGradient id="wire-gradient-and" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stop-color="#1890ff" />
          <stop offset="100%" stop-color="#0050b3" />
        </linearGradient>
        <linearGradient id="wire-gradient-or" x1="0%" y1="0%" x2="100%" y2="0%">
          <stop offset="0%" stop-color="#eb2f96" />
          <stop offset="100%" stop-color="#722ed1" />
        </linearGradient>
        <radialGradient id="gate-grad-and" cx="50%" cy="50%" r="50%">
          <stop offset="0%" stop-color="#0077b6" />
          <stop offset="100%" stop-color="#03045e" />
        </radialGradient>
        <radialGradient id="gate-grad-or" cx="50%" cy="50%" r="50%">
          <stop offset="0%" stop-color="#d946ef" />
          <stop offset="100%" stop-color="#701a75" />
        </radialGradient>
        <filter id="wire-glow">
          <feGaussianBlur stdDeviation="1" result="coloredBlur"/>
          <feMerge>
            <feMergeNode in="coloredBlur"/>
            <feMergeNode in="SourceGraphic"/>
          </feMerge>
        </filter>
      </defs>
      
      <!-- Connections / Bezier wires -->
      <path
        v-for="(wire, idx) in logicGateLayout.wires"
        :key="'wire-' + idx"
        :d="wire.d"
        :stroke="wire.color"
        stroke-width="1.5"
        fill="none"
        filter="url(#wire-glow)"
        opacity="0.8"
      />

      <!-- Logic Gates and Condition Badges -->
      <g v-for="elem in logicGateLayout.elements" :key="elem.id">
        <!-- Condition Nodes -->
        <g v-if="elem.type === 'condition'">
          <rect
            :x="elem.x"
            :y="elem.y - 10"
            width="85"
            height="20"
            rx="3"
            fill="#1e293b"
            stroke="#334155"
            stroke-width="1"
          />
          <text :x="elem.x + 4" :y="elem.y + 3" fill="#00b4d8" font-size="7" font-family="monospace" font-weight="bold">{{ elem.field }}</text>
          <text :x="elem.x + 45" :y="elem.y + 3" fill="#fa8c16" font-size="7" font-family="monospace">{{ elem.op }}</text>
          <text :x="elem.x + 58" :y="elem.y + 3" fill="#a78bfa" font-size="7" font-family="monospace">{{ elem.value }}</text>
        </g>

        <!-- Gate Nodes -->
        <g v-else>
          <circle
            :cx="elem.x"
            :cy="elem.y"
            r="13"
            :fill="elem.label === 'AND' ? 'url(#gate-grad-and)' : 'url(#gate-grad-or)'"
            :stroke="elem.label === 'AND' ? '#00b4d8' : '#f472b6'"
            stroke-width="1.5"
          />
          <text :x="elem.x" :y="elem.y - 1" text-anchor="middle" fill="#fff" font-size="8" font-family="monospace" font-weight="bold">{{ elem.label }}</text>
          <text :x="elem.x" :y="elem.y + 7" text-anchor="middle" fill="rgba(255,255,255,0.6)" font-size="5" font-family="monospace">GATE</text>
        </g>
      </g>
    </svg>
  </div>
</template>

<style scoped>
/* Logic gate visualizer styles */
.logic-gate-canvas {
  height: 180px;
  position: relative;
  border: 1px solid rgba(250, 140, 22, 0.2);
  background: rgba(13, 19, 33, 0.4);
  border-radius: 6px;
  overflow: hidden;
  box-shadow: inset 0 0 15px rgba(0, 0, 0, 0.5);
}
.logic-gate-grid {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background-image: 
    linear-gradient(to right, rgba(250, 140, 22, 0.05) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(250, 140, 22, 0.05) 1px, transparent 1px);
  background-size: 15px 15px;
  pointer-events: none;
}
.logic-gate-wires {
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 1;
}
</style>
