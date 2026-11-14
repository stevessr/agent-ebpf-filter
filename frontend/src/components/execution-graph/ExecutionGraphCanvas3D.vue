<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import type { ExecutionGraphEdge, ExecutionGraphNode } from "../../types/executionGraph";
import { buildDisplayGraph } from "./useDisplayGraphBuilder";
import { kindColor, nodeRadius, truncate } from "./useExecutionGraphHelpers";
import { harnessLabelNodes, harnessPosition } from "./harness3dLayout";

const props = withDefaults(defineProps<{
  nodes: ExecutionGraphNode[];
  edges: ExecutionGraphEdge[];
  selectedNodeId?: string;
  height?: number;
}>(), { selectedNodeId: "", height: 620 });

const emit = defineEmits<{
  (event: "select-node", id: string): void;
  (event: "unavailable"): void;
}>();
const host = ref<HTMLDivElement | null>(null);
const hovered = ref<ExecutionGraphNode | null>(null);
const hoverPosition = ref({ x: 0, y: 0 });
const displayCounts = ref({ nodes: 0, edges: 0 });
let renderer: THREE.WebGLRenderer | null = null;
let scene: THREE.Scene | null = null;
let camera: THREE.PerspectiveCamera | null = null;
let controls: OrbitControls | null = null;
let graphGroup: THREE.Group | null = null;
let nodeMesh: THREE.InstancedMesh | null = null;
let nodeIndex: ExecutionGraphNode[] = [];
let positionById = new Map<string, THREE.Vector3>();
let lastGraphSignature = "";
let labelLayer: HTMLDivElement | null = null;
let labelElements: Array<{ position: THREE.Vector3; element: HTMLDivElement }> = [];
let gridHelper: THREE.GridHelper | null = null;
let resizeObserver: ResizeObserver | null = null;
let intersectionObserver: IntersectionObserver | null = null;
let frame = 0;
let renderDirty = true;
let visible = true;
let disposed = false;
let downAt: { x: number; y: number } | null = null;

const raycaster = new THREE.Raycaster();
const pointer = new THREE.Vector2();
const reusable = new THREE.Object3D();
const reusableColor = new THREE.Color();

function positionFor(node: ExecutionGraphNode): THREE.Vector3 {
  const point = harnessPosition(node);
  return new THREE.Vector3(point.x, point.y, point.z);
}
function updateLabels() {
  if (!camera || !host.value || !labelLayer) return;
  const width = host.value.clientWidth;
  const height = host.value.clientHeight;
  for (const { position, element } of labelElements) {
    const projected = position.clone().project(camera);
    const inside = projected.z > -1 && projected.z < 1 &&
      projected.x >= -0.97 && projected.x <= 0.97 &&
      projected.y >= -0.95 && projected.y <= 0.95;
    element.hidden = !inside;
    if (!inside) continue;
    const x = (projected.x * 0.5 + 0.5) * width;
    const y = (1 - (projected.y * 0.5 + 0.5)) * height;
    element.style.transform = `translate(${Math.round(x)}px, ${Math.round(y)}px)`;
  }
}
function rebuildLabels() {
  if (!labelLayer) return;
  labelLayer.replaceChildren();
  labelElements = [];
  for (const node of harnessLabelNodes(nodeIndex, props.selectedNodeId || "")) {
    const position = positionById.get(node.id);
    if (!position) continue;
    const label = document.createElement("div");
    label.className = "harness-3d-node-label";
    label.textContent = truncate(node.label, 24);
    label.title = node.label;
    if (node.kind === "policy_alert" || (node.riskScore ?? 0) >= 60) {
      label.classList.add("harness-3d-node-label-risk");
    }
    if (node.id === props.selectedNodeId ||
        node.metadata?.sourceNodeId === props.selectedNodeId) {
      label.classList.add("harness-3d-node-label-selected");
    }
    labelLayer.appendChild(label);
    labelElements.push({ position, element: label });
  }
  updateLabels();
}
function updateSelection() {
  if (!nodeMesh) return;
  for (let i = 0; i < nodeIndex.length; i += 1) {
    const node = nodeIndex[i];
    const position = positionById.get(node.id);
    if (!position) continue;
    const selected = !!props.selectedNodeId &&
      (node.id === props.selectedNodeId ||
        node.metadata?.sourceNodeId === props.selectedNodeId);
    reusable.position.copy(position);
    reusable.scale.setScalar(Math.max(7, nodeRadius(node) * 0.7) * (selected ? 1.6 : 1));
    reusable.updateMatrix();
    nodeMesh.setMatrixAt(i, reusable.matrix);
  }
  nodeMesh.instanceMatrix.needsUpdate = true;
  rebuildLabels();
  requestRender();
}
function graphSignature(nodes: ExecutionGraphNode[], edges: ExecutionGraphEdge[]): string {
  // Include labels/risks to avoid stale overlays on live metadata updates,
  // without rebuilding GPU objects on truly identical snapshots.
  return nodes.map((node) =>
    [node.id, node.kind, node.label, node.riskScore,
     node.metadata?.sourceNodeId, node.metadata?.eventCount].join(":"),
  ).join("\\n") + "|" +
    edges.map((edge) => [edge.id, edge.kind, edge.source, edge.target].join(":")).join("\\n");
}
function disposeGroup(group: THREE.Group | null) {
  if (!group) return;
  group.traverse((object) => {
    const drawable = object as THREE.Mesh | THREE.LineSegments;
    drawable.geometry?.dispose();
    const materials = drawable.material;
    if (Array.isArray(materials)) materials.forEach((item) => item.dispose());
    else materials?.dispose();
  });
  group.parent?.remove(group);
}
function redraw() {
  if (!scene) return;
  const graph = buildDisplayGraph(props.nodes, props.edges);
  const signature = graphSignature(graph.nodes, graph.edges);
  if (signature === lastGraphSignature && nodeMesh) return;
  lastGraphSignature = signature;
  disposeGroup(graphGroup);
  graphGroup = new THREE.Group();
  scene.add(graphGroup);
  displayCounts.value = { nodes: graph.nodes.length, edges: graph.edges.length };
  nodeIndex = graph.nodes;
  nodeMesh = null;
  positionById = new Map(graph.nodes.map((node) => [node.id, positionFor(node)]));
  if (!graph.nodes.length) {
    rebuildLabels();
    requestRender();
    return;
  }

  const positions = positionById;
  const geometry = new THREE.SphereGeometry(1, 12, 9);
  const material = new THREE.MeshStandardMaterial({
    color: "#ffffff", roughness: 0.38, metalness: 0.16,
  });
  const mesh = new THREE.InstancedMesh(geometry, material, graph.nodes.length);
  mesh.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  mesh.frustumCulled = false;
  for (let i = 0; i < graph.nodes.length; i++) {
    const node = graph.nodes[i];
    const selected = node.id === props.selectedNodeId ||
      node.metadata?.sourceNodeId === props.selectedNodeId;
    const risk = node.riskScore ?? 0;
    const radius = Math.max(7, nodeRadius(node) * 0.7) * (selected ? 1.6 : 1);
    reusable.position.copy(positions.get(node.id)!);
    reusable.scale.setScalar(radius);
    reusable.updateMatrix();
    mesh.setMatrixAt(i, reusable.matrix);
    const color = risk >= 60 ? "#ef4444" : kindColor(node.kind);
    mesh.setColorAt(i, reusableColor.set(color));
  }
  mesh.instanceMatrix.needsUpdate = true;
  if (mesh.instanceColor) mesh.instanceColor.needsUpdate = true;
  graphGroup.add(mesh);
  nodeMesh = mesh;

  const normal: number[] = [];
  const critical: number[] = [];
  for (const edge of graph.edges) {
    const start = positions.get(edge.source);
    const end = positions.get(edge.target);
    if (!start || !end) continue;
    const target = /^(blocked|alerted|rewritten)$/.test(edge.kind) ? critical : normal;
    target.push(start.x, start.y, start.z, end.x, end.y, end.z);
  }
  function addLines(positions: number[], color: string, opacity: number) {
    if (!positions.length) return;
    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute("position",
      new THREE.Float32BufferAttribute(positions, 3));
    const lines = new THREE.LineSegments(geometry, new THREE.LineBasicMaterial({
      color, transparent: true, opacity, depthWrite: false,
    }));
    graphGroup!.add(lines);
  }
  addLines(normal, "#64748b", 0.35);
  addLines(critical, "#fb7185", 0.95);

  // Causal direction matters for a harness: a link to an endpoint is not
  // equivalent to the reverse flow. A single instanced draw call keeps
  // directional markers affordable on live 1000+ edge snapshots.
  const isCriticalEdge = (edge: ExecutionGraphEdge) =>
    /^(blocked|alerted|rewritten)$/.test(edge.kind);
  const directedEdges = graph.edges
    .filter((edge) => positions.has(edge.source) && positions.has(edge.target))
    .sort((a, b) => Number(isCriticalEdge(b)) - Number(isCriticalEdge(a)) ||
      a.id.localeCompare(b.id))
    .slice(0, 1200);
  const nodeById = new Map(nodeIndex.map((node) => [node.id, node]));
  if (directedEdges.length) {
    const arrowGeometry = new THREE.ConeGeometry(2.4, 8, 6);
    const arrowMaterial = new THREE.MeshBasicMaterial({
      color: "#ffffff", transparent: true, opacity: 0.85,
    });
    const arrows = new THREE.InstancedMesh(
      arrowGeometry, arrowMaterial, directedEdges.length,
    );
    arrows.frustumCulled = false;
    const defaultAxis = new THREE.Vector3(0, 1, 0);
    const arrow = new THREE.Object3D();
    for (let i = 0; i < directedEdges.length; i++) {
      const edge = directedEdges[i];
      const start = positions.get(edge.source)!;
      const end = positions.get(edge.target)!;
      const delta = end.clone().sub(start);
      const distance = delta.length();
      if (distance < 0.001) {
        arrow.position.copy(end);
        arrow.scale.setScalar(0);
      } else {
        const direction = delta.multiplyScalar(1 / distance);
        const targetRadius = Math.max(7, nodeRadius(nodeById.get(edge.target) || {
          id: edge.target, kind: "unknown", label: "",
        }) * 0.7);
        arrow.position.copy(end).addScaledVector(direction,
          -Math.min(distance * 0.65, targetRadius + 10));
        arrow.quaternion.setFromUnitVectors(defaultAxis, direction);
        arrow.scale.setScalar(1);
      }
      arrow.updateMatrix();
      arrows.setMatrixAt(i, arrow.matrix);
      arrows.setColorAt(
        i,
        reusableColor.set(
          /^(blocked|alerted|rewritten)$/.test(edge.kind) ? "#fb7185" : "#94a3b8",
        ),
      );
    }
    arrows.instanceMatrix.needsUpdate = true;
    if (arrows.instanceColor) arrows.instanceColor.needsUpdate = true;
    graphGroup.add(arrows);
  }

  rebuildLabels();
  requestRender();
}

function setSize() {
  if (!renderer || !camera || !host.value) return;
  const width = Math.max(1, host.value.clientWidth);
  const height = Math.max(1, host.value.clientHeight);
  renderer.setSize(width, height, false);
  camera.aspect = width / height;
  camera.updateProjectionMatrix();
  requestRender();
}
function renderLoop() {
  frame = 0;
  if (disposed || !visible || !renderer || !scene || !camera) return;
  const moved = controls?.update() ?? false;
  if (moved || renderDirty) {
    renderer.render(scene, camera);
    updateLabels();
    renderDirty = false;
  }
  // OrbitControls reports continued damping updates. Stop requesting
  // frames as soon as the camera and scene settle.
  if (moved) beginLoop();
}
function beginLoop() {
  if (frame || disposed || !visible) return;
  frame = requestAnimationFrame(renderLoop);
}
function requestRender() {
  renderDirty = true;
  beginLoop();
}
function stopLoop() {
  if (!frame) return;
  cancelAnimationFrame(frame);
  frame = 0;
}
function intersect(event: PointerEvent): ExecutionGraphNode | null {
  if (!renderer || !camera || !nodeMesh) return null;
  const rect = renderer.domElement.getBoundingClientRect();
  if (!rect.width || !rect.height) return null;
  pointer.set(
    ((event.clientX - rect.left) / rect.width) * 2 - 1,
    -((event.clientY - rect.top) / rect.height) * 2 + 1,
  );
  raycaster.setFromCamera(pointer, camera);
  const result = raycaster.intersectObject(nodeMesh)[0];
  return result?.instanceId === undefined
    ? null : (nodeIndex[result.instanceId] ?? null);
}
function onMove(event: PointerEvent) {
  hovered.value = intersect(event);
  if (!host.value) return;
  const rect = host.value.getBoundingClientRect();
  hoverPosition.value = {
    x: Math.min(rect.width - 180, Math.max(12, event.clientX - rect.left + 12)),
    y: Math.min(rect.height - 70, Math.max(12, event.clientY - rect.top + 12)),
  };
  if (renderer) renderer.domElement.style.cursor = hovered.value ? "pointer" : "grab";
}
function onLeave() {
  hovered.value = null;
  downAt = null;
}
function onDown(event: PointerEvent) {
  downAt = { x: event.clientX, y: event.clientY };
}
function onUp(event: PointerEvent) {
  if (!downAt || Math.hypot(event.clientX - downAt.x, event.clientY - downAt.y) > 5) {
    downAt = null;
    return;
  }
  downAt = null;
  const node = intersect(event);
  if (node) emit("select-node", node.metadata?.sourceNodeId || node.id);
}
function onContextLost(event: Event) {
  event.preventDefault();
  emit("unavailable");
}
function resetCamera() {
  if (!camera || !controls) return;
  camera.position.set(100, 300, 1200);
  controls.target.set(0, 0, 0);
  controls.update();
  requestRender();
}
function focusSelected() {
  if (!controls || !camera) return;
  const selected = nodeIndex.find((node) =>
    node.id === props.selectedNodeId ||
    node.metadata?.sourceNodeId === props.selectedNodeId,
  );
  const target = selected ? positionById.get(selected.id) : null;
  if (!target) return;
  const offset = camera.position.clone().sub(controls.target);
  controls.target.copy(target);
  camera.position.copy(target).add(offset);
  controls.update();
  requestRender();
}
async function init() {
  await nextTick();
  if (disposed || !host.value || renderer) return;
  try {
    const canvas = document.createElement("canvas");
    const available = !!(canvas.getContext("webgl2") || canvas.getContext("webgl"));
    if (!available) throw new Error("WebGL unavailable");
    renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false, powerPreference: "low-power" });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.5));
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    host.value.appendChild(renderer.domElement);
    renderer.domElement.addEventListener("pointermove", onMove);
    renderer.domElement.addEventListener("pointerdown", onDown);
    renderer.domElement.addEventListener("pointerup", onUp);
    renderer.domElement.addEventListener("pointerleave", onLeave);
    labelLayer = document.createElement("div");
    labelLayer.className = "harness-3d-label-layer";
    host.value.appendChild(labelLayer);
    renderer.domElement.addEventListener("webglcontextlost", onContextLost);
    scene = new THREE.Scene();
    scene.background = new THREE.Color("#0b1220");
    scene.fog = new THREE.Fog("#0b1220", 900, 2600);
    scene.add(new THREE.AmbientLight("#e2e8f0", 2.1));
    const key = new THREE.DirectionalLight("#bfdbfe", 3.2);
    key.position.set(-180, 420, 550);
    scene.add(key);
    gridHelper = new THREE.GridHelper(1550, 24, "#334155", "#1e293b");
    gridHelper.position.y = -290;
    scene.add(gridHelper);
    camera = new THREE.PerspectiveCamera(55, 1, 1, 4000);
    camera.position.set(100, 300, 1200);
    controls = new OrbitControls(camera, renderer.domElement);
    controls.enableDamping = true;
    controls.dampingFactor = 0.07;
    controls.minDistance = 250;
    controls.maxDistance = 2400;
    controls.target.set(0, 0, 0);
    controls.addEventListener("change", requestRender);
    setSize();
    redraw();
    resizeObserver = new ResizeObserver(setSize);
    resizeObserver.observe(host.value);
    intersectionObserver = new IntersectionObserver((entries) => {
      visible = entries.some((entry) => entry.isIntersecting);
      if (visible) requestRender();
      else stopLoop();
    });
    intersectionObserver.observe(host.value);
    beginLoop();
  } catch {
    emit("unavailable");
  }
}
watch(() => [props.nodes, props.edges], redraw);
watch(() => props.selectedNodeId, updateSelection);
onMounted(init);
onBeforeUnmount(() => {
  disposed = true;
  stopLoop();
  resizeObserver?.disconnect();
  intersectionObserver?.disconnect();
  controls?.removeEventListener("change", requestRender);
  controls?.dispose();
  if (renderer) {
    renderer.domElement.removeEventListener("pointermove", onMove);
    renderer.domElement.removeEventListener("pointerdown", onDown);
    renderer.domElement.removeEventListener("pointerup", onUp);
    renderer.domElement.removeEventListener("pointerleave", onLeave);
    renderer.domElement.removeEventListener("webglcontextlost", onContextLost);
  }
  disposeGroup(graphGroup);
  if (gridHelper) {
    gridHelper.geometry.dispose();
    const material = gridHelper.material;
    if (Array.isArray(material)) material.forEach((item) => item.dispose());
    else material.dispose();
    gridHelper.parent?.remove(gridHelper);
    gridHelper = null;
  }
  labelLayer?.remove();
  labelLayer = null;
  labelElements = [];
  renderer?.dispose();
  renderer?.domElement.remove();
  renderer = null;
  nodeMesh = null;
  graphGroup = null;
  scene = null;
  camera = null;
});
</script>

<template>
  <div ref="host" class="harness-3d" :style="{ height: height + 'px' }">
    <div class="harness-3d-overlay">
      <strong>HARNESS · 3D</strong>
      <span>{{ displayCounts.nodes }} nodes · {{ displayCounts.edges }} links</span>
      <small v-if="displayCounts.edges > 1200">箭头上限 1200 条，告警关系优先</small>
      <small>箭头表示因果方向 · 拖动旋转 · 滚轮缩放 · 点击查看证据</small>
    </div>
    <div class="harness-3d-controls">
      <button type="button" @click="focusSelected" :disabled="!selectedNodeId">定位选中</button>
      <button type="button" @click="resetCamera">重置视角</button>
    </div>
    <div
      v-if="hovered"
      class="harness-3d-tooltip"
      :style="{ left: hoverPosition.x + 'px', top: hoverPosition.y + 'px' }"
    >
      <b>{{ truncate(hovered.label, 44) }}</b>
      <small>{{ hovered.kind }} · risk {{ hovered.riskScore ?? 0 }}</small>
    </div>
    <div v-if="!displayCounts.nodes" class="harness-3d-empty">暂无可显示的捕获行为</div>
  </div>
</template>

<style scoped>
.harness-3d {
  width: 100%;
  min-height: 360px;
  position: relative;
  overflow: hidden;
  border: 1px solid #243348;
  border-radius: 12px;
  background: #0b1220;
}
.harness-3d :deep(canvas) { display: block; width: 100%; height: 100%; }
.harness-3d-overlay {
  position: absolute; z-index: 2; top: 16px; left: 18px;
  display: flex; flex-direction: column; gap: 3px;
  color: #e2e8f0; pointer-events: none; text-shadow: 0 1px 3px #020617;
}
.harness-3d-overlay strong { font-size: 12px; letter-spacing: .13em; }
.harness-3d-overlay span { font-size: 11px; color: #94a3b8; }
.harness-3d-overlay small { font-size: 10px; color: #64748b; }
.harness-3d-controls {
  position: absolute; z-index: 4; top: 12px; right: 12px;
  display: flex; gap: 6px;
}
.harness-3d-controls button {
  border-radius: 7px; border: 1px solid #475569;
  background: #1e293b; color: #e2e8f0; padding: 5px 9px;
  font-size: 11px; cursor: pointer;
}
.harness-3d-controls button:disabled { opacity: .4; cursor: default; }
.harness-3d-controls button:not(:disabled):hover { background: #334155; }
.harness-3d :deep(.harness-3d-label-layer) {
  position: absolute; inset: 0; pointer-events: none; overflow: hidden;
}
.harness-3d :deep(.harness-3d-node-label) {
  position: absolute; top: 0; left: 0; color: #dbeafe;
  font: 10px/1.3 system-ui, sans-serif; white-space: nowrap;
  padding: 2px 5px; border-radius: 4px; background: #0f172aca;
  border: 1px solid #334155; max-width: 160px; overflow: hidden;
  text-overflow: ellipsis;
}
.harness-3d :deep(.harness-3d-node-label-risk) {
  color: #fecdd3; border-color: #be123c;
}
.harness-3d :deep(.harness-3d-node-label-selected) {
  color: #fef08a; border-color: #eab308; font-weight: 700;
}
.harness-3d-tooltip {
  position: absolute; z-index: 3; pointer-events: none;
  display: flex; flex-direction: column; gap: 3px;
  max-width: 230px; padding: 8px 10px; border-radius: 8px;
  color: #f1f5f9; background: rgba(15, 23, 42, .95);
  border: 1px solid #475569; box-shadow: 0 10px 24px #0005;
}
.harness-3d-tooltip b { font-size: 12px; overflow-wrap: anywhere; }
.harness-3d-tooltip small { color: #94a3b8; }
.harness-3d-empty {
  position: absolute; inset: 0; display: grid; place-items: center;
  color: #94a3b8; font-size: 13px; pointer-events: none;
}
</style>
