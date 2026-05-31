<script setup lang="ts">
import { ref, toRef } from "vue";
import {
  useTrafficGraph,
  type TrafficInterface,
} from "../../composables/network/useTrafficGraph";

const props = withDefaults(
  defineProps<{
    interfaces: TrafficInterface[];
    height?: number;
  }>(),
  {
    height: 420,
  },
);

const emit = defineEmits<{
  (event: "select-interface", name: string): void;
}>();

const containerRef = ref<HTMLElement | null>(null);
const svgRef = ref<SVGSVGElement | null>(null);

useTrafficGraph(
  containerRef,
  svgRef,
  toRef(props, "interfaces"),
  toRef(props, "height"),
  (name) => emit("select-interface", name),
);
</script>

<template>
  <div ref="containerRef" class="traffic-graph">
    <svg ref="svgRef" class="traffic-graph__svg" />
  </div>
</template>

<style scoped>
.traffic-graph {
  width: 100%;
  min-height: 200px;
  border-radius: 12px;
  overflow: hidden;
  background: radial-gradient(
    circle at top,
    #ffffff 0%,
    #f8fbff 45%,
    #eef4ff 100%
  );
  border: 1px solid #e5eefb;
  position: relative;
}
.traffic-graph__svg {
  width: 100%;
  display: block;
}
:deep(.traffic-link) {
  stroke-dasharray: 16 14;
  animation: traffic-flow 0.5s linear infinite;
  opacity: 0.8;
  pointer-events: none;
}
:deep(.traffic-link-arrow) {
  pointer-events: none;
}
:deep(.traffic-node) {
  touch-action: none;
  user-select: none;
}
:deep(.traffic-node circle) {
  stroke: rgba(255, 255, 255, 0.96);
  stroke-width: 2.5px;
  filter: drop-shadow(0 10px 18px rgba(15, 23, 42, 0.16));
}
:deep(.traffic-node text) {
  fill: #0f172a;
  paint-order: stroke;
  stroke: rgba(255, 255, 255, 0.95);
  stroke-width: 4px;
  stroke-linejoin: round;
}
@keyframes traffic-flow {
  to {
    stroke-dashoffset: -60;
  }
}
</style>
