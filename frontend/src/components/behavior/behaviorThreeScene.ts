import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import type { AgentEvent } from "../../composables/dashboard/dashboardConstants";
import { isAttentionEvent, type BehaviorLane } from "../../composables/behavior/behaviorModel";
import { boundedThreeEvents, eventAngle, localOrbitPoint, laneHeight, laneRadius, THREE_CAMERA, THREE_TARGET } from "./behaviorThreeLayout";

export interface ThreePalette { canvas: string; surface: string; border: string; text: string; accent: string; warning: string }
export interface ThreeInput {
  events: readonly AgentEvent[]; now: number; windowMs: number; activeLane: BehaviorLane | "all";
  selectedKey?: string; paused: boolean; reducedMotion: boolean; autoOrbit: boolean;
  expanded?: boolean; showTrails?: boolean; playbackRate?: number;
}
interface Mark { event: AgentEvent; index: number; attention: boolean; mesh: THREE.Mesh; trail: THREE.Line; offset: number }

/** Owns imperative WebGL objects. Vue owns the event source and all inspection state. */
export class BehaviorThreeScene {
  readonly renderer: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera = new THREE.PerspectiveCamera(42, 1, .1, 100);
  private controls: OrbitControls;
  private raycaster = new THREE.Raycaster();
  private pointer = new THREE.Vector2();
  private groups: THREE.Group[] = [];
  private tracks: THREE.LineBasicMaterial[] = [];
  private marks = new Map<string, Mark>();
  private ownedGeometry = new Set<THREE.BufferGeometry>();
  private ownedMaterials = new Set<THREE.Material>();
  private ordinary: THREE.MeshStandardMaterial;
  private warning: THREE.MeshStandardMaterial;
  private dimOrdinary: THREE.MeshStandardMaterial;
  private dimWarning: THREE.MeshStandardMaterial;
  private wire: THREE.MeshBasicMaterial;
  private sphere: THREE.SphereGeometry;
  private diamond: THREE.OctahedronGeometry;
  private selection: THREE.Mesh;
  private grid: THREE.GridHelper;
  private core: THREE.Group;
  private input: ThreeInput | null = null;
  private palette: ThreePalette;
  private active = true;
  private disposed = false;
  private looping = false;
  private lastFrame = 0;
  private updatedAt = 0;
  private phase = 0;
  private down: { x: number; y: number; id: number } | null = null;
  private hoverKey = "";
  private onPick: (event: AgentEvent) => void;
  private onHover: (event: AgentEvent | null, x: number, y: number) => void;

  constructor(host: HTMLElement, palette: ThreePalette, onPick: (event: AgentEvent) => void, onHover: (event: AgentEvent | null, x: number, y: number) => void) {
    this.palette = palette; this.onPick = onPick; this.onHover = onHover;
    this.renderer = new THREE.WebGLRenderer({ antialias: true, alpha: false, powerPreference: "low-power" });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 1.75));
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    const canvas = this.renderer.domElement;
    canvas.tabIndex = 0; canvas.setAttribute("role", "img");
    canvas.dataset.renderer = "three";
    host.appendChild(canvas);
    this.camera.position.set(...THREE_CAMERA);
    this.controls = new OrbitControls(this.camera, canvas);
    this.controls.target.set(...THREE_TARGET);
    this.controls.minDistance = 7; this.controls.maxDistance = 22;
    this.controls.maxPolarAngle = Math.PI * .84; this.controls.minPolarAngle = .12;
    this.controls.enablePan = false; this.controls.autoRotateSpeed = .45;
    this.controls.addEventListener("change", this.renderOnce);
    this.scene.add(new THREE.HemisphereLight(0xffffff, 0x718096, 2.6));
    const light = new THREE.DirectionalLight(0xffffff, 3); light.position.set(3, 8, 5); this.scene.add(light);
    const material = <T extends THREE.Material>(m: T): T => { this.ownedMaterials.add(m); return m; };
    const geometry = <T extends THREE.BufferGeometry>(g: T): T => { this.ownedGeometry.add(g); return g; };
    this.ordinary = material(new THREE.MeshStandardMaterial({ color: palette.accent, roughness: .38, metalness: .15 }));
    this.warning = material(new THREE.MeshStandardMaterial({ color: palette.warning, roughness: .35, metalness: .1 }));
    this.dimOrdinary = material(this.ordinary.clone()); this.dimOrdinary.transparent = true; this.dimOrdinary.opacity = .18;
    this.dimWarning = material(this.warning.clone()); this.dimWarning.transparent = true; this.dimWarning.opacity = .18;
    this.sphere = geometry(new THREE.SphereGeometry(.075, 12, 8));
    this.diamond = geometry(new THREE.OctahedronGeometry(.115, 0));
    this.wire = material(new THREE.MeshBasicMaterial({ color: palette.accent, wireframe: true, transparent: true, opacity: .65 }));
    this.core = new THREE.Group();
    this.core.add(new THREE.Mesh(geometry(new THREE.IcosahedronGeometry(.82, 1)), this.wire));
    const coreBody = material(new THREE.MeshStandardMaterial({ color: palette.surface, roughness: .7, metalness: .18 }));
    this.core.add(new THREE.Mesh(geometry(new THREE.IcosahedronGeometry(.8, 1)), coreBody));
    for (let i = 0; i < 3; i++) {
      const ring = new THREE.Mesh(geometry(new THREE.TorusGeometry(1.07 + i * .12, .013, 6, 96)), this.wire);
      ring.rotation.set(Math.PI / 2 + i * .62, i * .4, i * .5); this.core.add(ring);
    }
    this.scene.add(this.core);
    this.selection = new THREE.Mesh(geometry(new THREE.SphereGeometry(.15, 12, 8)), material(new THREE.MeshBasicMaterial({ color: palette.text, wireframe: true })));
    this.selection.visible = false; this.scene.add(this.selection);
    for (let i = 0; i < 4; i++) {
      const group = new THREE.Group(); group.position.y = laneHeight(i); group.rotation.z = (i - 1.5) * .1;
      for (const offset of [-.09, 0, .09]) {
        const points = Array.from({ length: 180 }, (_, n) => new THREE.Vector3(...localOrbitPoint(i, n / 180 * Math.PI * 2, offset)));
        const trackMaterial = material(new THREE.LineBasicMaterial({ color: palette.border, transparent: true, opacity: offset === 0 ? 1 : .45 }));
        this.tracks.push(trackMaterial);
        group.add(new THREE.LineLoop(geometry(new THREE.BufferGeometry().setFromPoints(points)), trackMaterial));
      }
      const ticks: THREE.Vector3[] = [];
      for (let tick = 0; tick < 60; tick++) {
        const angle = tick / 60 * Math.PI * 2;
        ticks.push(new THREE.Vector3(...localOrbitPoint(i, angle, .12)), new THREE.Vector3(...localOrbitPoint(i, angle, tick % 5 === 0 ? .25 : .17)));
      }
      const tickMaterial = material(new THREE.LineBasicMaterial({ color: palette.accent, transparent: true, opacity: .4 }));
      tickMaterial.userData.orbitTicks = true;
      group.add(new THREE.LineSegments(geometry(new THREE.BufferGeometry().setFromPoints(ticks)), tickMaterial));
      this.groups.push(group); this.scene.add(group);
    }
    this.grid = new THREE.GridHelper(16, 32, palette.border, palette.border);
    this.grid.position.y = -1.8;
    for (const m of [this.grid.material].flat()) { m.transparent = true; m.opacity = .32; this.ownedMaterials.add(m); }
    this.ownedGeometry.add(this.grid.geometry); this.scene.add(this.grid);
    this.setPalette(palette);
    canvas.addEventListener("pointerdown", this.pointerDown);
    canvas.addEventListener("pointerup", this.pointerUp);
    canvas.addEventListener("pointermove", this.pointerMove);
    canvas.addEventListener("pointerleave", this.pointerLeave);
    canvas.addEventListener("pointercancel", this.pointerLeave);
    canvas.addEventListener("keydown", this.keyDown);
  }

  setPalette(palette: ThreePalette): void {
    this.palette = palette; this.renderer.setClearColor(palette.canvas);
    this.ordinary.color.set(palette.accent); this.warning.color.set(palette.warning);
    this.dimOrdinary.color.set(palette.accent); this.dimWarning.color.set(palette.warning);
    this.wire.color.set(palette.accent);
    for (const m of this.tracks) m.color.set(palette.border).lerp(new THREE.Color(palette.accent), .22);
    for (const m of this.ownedMaterials) if (m instanceof THREE.LineBasicMaterial && m.userData.orbitTicks) m.color.set(palette.accent);
    (this.selection.material as THREE.MeshBasicMaterial).color.set(palette.text);
    this.core.traverse(object => { if (object instanceof THREE.Mesh && object.material instanceof THREE.MeshStandardMaterial) object.material.color.set(palette.surface); });
    const gridColor = new THREE.Color(palette.border);
    const gridColors = this.grid.geometry.getAttribute("color") as THREE.BufferAttribute;
    for (let i = 0; i < gridColors.count; i++) gridColors.setXYZ(i, gridColor.r, gridColor.g, gridColor.b);
    gridColors.needsUpdate = true;
    for (const m of [this.grid.material].flat()) { (m as THREE.LineBasicMaterial).color.set(0xffffff); m.opacity = .24; }
    for (const mark of this.marks.values()) (mark.trail.material as THREE.LineBasicMaterial).color.set(mark.attention ? palette.warning : palette.accent);
    this.renderOnce();
  }

  update(input: ThreeInput): void {
    if (this.disposed) return;
    this.input = input; this.updatedAt = performance.now();
    this.groups.forEach((group, index) => { group.position.y = laneHeight(index) * (input.expanded ? 2.5 : 1); });
    this.grid.position.y = input.expanded ? -3.4 : -1.8;
    const visible = boundedThreeEvents(input.events, input.selectedKey);
    const keys = new Set(visible.map(item => item.event.key));
    for (const [key, mark] of this.marks) if (!keys.has(key)) {
      mark.mesh.removeFromParent(); mark.trail.removeFromParent(); mark.trail.geometry.dispose(); (mark.trail.material as THREE.Material).dispose(); this.marks.delete(key);
    }
    for (const { event, index, lane } of visible) {
      const attention = isAttentionEvent(event);
      let mark = this.marks.get(event.key);
      if (mark && (mark.index !== index || mark.attention !== attention)) {
        mark.mesh.removeFromParent(); mark.trail.removeFromParent(); mark.trail.geometry.dispose(); (mark.trail.material as THREE.Material).dispose(); this.marks.delete(event.key); mark = undefined;
      }
      const dim = input.activeLane !== "all" && input.activeLane !== lane;
      if (!mark) {
        const mesh = new THREE.Mesh(attention ? this.diamond : this.sphere, attention ? this.warning : this.ordinary);
        const trail = new THREE.Line(new THREE.BufferGeometry().setAttribute("position", new THREE.BufferAttribute(new Float32Array(36), 3)), new THREE.LineBasicMaterial({ color: attention ? this.palette.warning : this.palette.accent, transparent: true, opacity: .35 }));
        mark = { event, index, attention, mesh, trail, offset: 0 };
        this.groups[index]!.add(mesh, trail); this.marks.set(event.key, mark);
      }
      mark.event = event; mark.mesh.userData.event = event;
      mark.trail.visible = input.showTrails !== false;
      mark.mesh.material = attention ? (dim ? this.dimWarning : this.warning) : (dim ? this.dimOrdinary : this.ordinary);
      (mark.trail.material as THREE.LineBasicMaterial).opacity = dim ? .06 : .36;
      mark.offset = ((Number.parseInt(event.key.slice(-2), 36) || 0) % 3 - 1) * .025;
    }
    this.renderer.domElement.dataset.nodeCount = String(this.marks.size);
    this.controls.enableDamping = !input.reducedMotion && !input.paused;
    this.controls.autoRotate = input.autoOrbit && !input.paused && !input.reducedMotion;
    this.positionMarks(input.now); this.schedule(); this.renderOnce();
  }

  resize(width: number, height: number): void {
    if (this.disposed || width <= 0 || height <= 0) return;
    this.renderer.setSize(width, height, false); this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix(); this.renderOnce();
  }
  setActive(active: boolean): void { this.active = active; this.schedule(); if (active) this.renderOnce(); }
  reset(): void {
    this.camera.position.set(...THREE_CAMERA); this.controls.target.set(...THREE_TARGET);
    this.controls.update(); this.renderOnce();
  }
  setView(view: "perspective" | "top" | "side"): void {
    this.controls.target.set(...THREE_TARGET);
    if (view === "top") this.camera.position.set(.01, 14, 1.8);
    else if (view === "side") this.camera.position.set(13, 3, 2);
    else this.camera.position.set(...THREE_CAMERA);
    this.controls.update(); this.renderOnce();
  }
  focusSelected(): void {
    const key = this.input?.selectedKey; const mark = key ? this.marks.get(key) : undefined;
    if (!mark) return;
    mark.mesh.updateWorldMatrix(true, false);
    const target = mark.mesh.getWorldPosition(new THREE.Vector3());
    const direction = this.camera.position.clone().sub(this.controls.target).normalize().multiplyScalar(7);
    this.controls.target.copy(target); this.camera.position.copy(target).add(direction);
    this.controls.update(); this.renderOnce();
  }
  private positionMarks(now: number): void {
    const input = this.input; if (!input) return;
    this.selection.visible = false;
    for (const [key, mark] of this.marks) {
      const angle = eventAngle(now, input.windowMs, mark.event.receivedAtMs ?? now);
      mark.mesh.position.set(...localOrbitPoint(mark.index, angle, mark.offset));
      const recent = now - (mark.event.receivedAtMs ?? 0) < 5000;
      const scale = key === input.selectedKey ? 1.55 : (recent && !input.paused && !input.reducedMotion ? 1 + Math.sin(this.phase * 2.5) * .09 : 1);
      mark.mesh.scale.setScalar(scale);
      const positions = mark.trail.geometry.getAttribute("position") as THREE.BufferAttribute;
      for (let n = 0; n < 12; n++) positions.setXYZ(n, ...localOrbitPoint(mark.index, angle - n * .012, mark.offset));
      positions.needsUpdate = true; mark.trail.geometry.computeBoundingSphere();
      if (key === input.selectedKey) { mark.mesh.updateWorldMatrix(true, false); mark.mesh.getWorldPosition(this.selection.position); this.selection.visible = true; }
    }
  }
  private schedule(): void {
    if (this.disposed) return;
    const animate = this.active && !!this.input && !this.input.paused && !this.input.reducedMotion;
    if (animate === this.looping) return;
    this.looping = animate; this.lastFrame = 0;
    this.renderer.setAnimationLoop(animate ? this.frame : null);
    if (this.active) this.renderOnce();
  }
  private frame = (timestamp: number): void => {
    if (this.disposed || !this.input) return;
    if (timestamp - this.lastFrame < 1000 / 30) return;
    const delta = this.lastFrame ? Math.min(.1, (timestamp - this.lastFrame) / 1000) : 0;
    this.lastFrame = timestamp; this.phase += delta;
    this.positionMarks(this.input.now + Math.min(1000, Math.max(0, performance.now() - this.updatedAt)) * (this.input.playbackRate ?? 1));
    this.core.rotation.y += delta * .08;
    this.controls.update(delta); this.renderOnce();
  };
  private renderOnce = (): void => { if (!this.disposed && this.active) this.renderer.render(this.scene, this.camera); };
  private hit(event: PointerEvent): AgentEvent | null {
    const rect = this.renderer.domElement.getBoundingClientRect();
    if (!rect.width || !rect.height) return null;
    this.pointer.set((event.clientX - rect.left) / rect.width * 2 - 1, -(event.clientY - rect.top) / rect.height * 2 + 1);
    this.scene.updateMatrixWorld(true); this.camera.updateMatrixWorld();
    this.raycaster.setFromCamera(this.pointer, this.camera);
    const hit = this.raycaster.intersectObjects([...this.marks.values()].map(mark => mark.mesh), false)[0];
    return hit ? (hit.object.userData.event as AgentEvent) : null;
  }
  private pointerDown = (event: PointerEvent): void => { if (event.button === 0) this.down = { x: event.clientX, y: event.clientY, id: event.pointerId }; };
  private pointerUp = (event: PointerEvent): void => {
    const down = this.down; this.down = null;
    if (!down || down.id !== event.pointerId || Math.hypot(event.clientX - down.x, event.clientY - down.y) > 6) return;
    const hit = this.hit(event); if (hit) this.onPick(hit);
  };
  private pointerMove = (event: PointerEvent): void => {
    if (this.down) return;
    const hit = this.hit(event); const rect = this.renderer.domElement.getBoundingClientRect();
    this.renderer.domElement.style.cursor = hit ? "pointer" : "grab";
    if ((hit?.key ?? "") !== this.hoverKey) { this.hoverKey = hit?.key ?? ""; this.onHover(hit, Math.min(rect.width - 180, Math.max(8, event.clientX - rect.left + 14)), Math.max(8, event.clientY - rect.top - 54)); }
  };
  private pointerLeave = (): void => { this.down = null; this.hoverKey = ""; this.onHover(null, 0, 0); };
  private keyDown = (event: KeyboardEvent): void => {
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    const keys = ["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown", "+", "=", "-", "r", "R"];
    if (!keys.includes(event.key)) return;
    event.preventDefault();
    if (event.key.toLowerCase() === "r") { this.reset(); return; }
    const offset = this.camera.position.clone().sub(this.controls.target);
    const sphere = new THREE.Spherical().setFromVector3(offset);
    if (event.key === "ArrowLeft") sphere.theta -= .12;
    if (event.key === "ArrowRight") sphere.theta += .12;
    if (event.key === "ArrowUp") sphere.phi = Math.max(.12, sphere.phi - .12);
    if (event.key === "ArrowDown") sphere.phi = Math.min(Math.PI * .84, sphere.phi + .12);
    if (["+", "="].includes(event.key)) sphere.radius = Math.max(7, sphere.radius * .9);
    if (event.key === "-") sphere.radius = Math.min(22, sphere.radius * 1.1);
    this.camera.position.copy(this.controls.target).add(new THREE.Vector3().setFromSpherical(sphere));
    this.controls.update(); this.renderOnce();
  };
  dispose(): void {
    if (this.disposed) return; this.disposed = true;
    this.renderer.setAnimationLoop(null); this.controls.removeEventListener("change", this.renderOnce); this.controls.dispose();
    const canvas = this.renderer.domElement;
    canvas.removeEventListener("pointerdown", this.pointerDown); canvas.removeEventListener("pointerup", this.pointerUp);
    canvas.removeEventListener("pointermove", this.pointerMove); canvas.removeEventListener("pointerleave", this.pointerLeave); canvas.removeEventListener("pointercancel", this.pointerLeave); canvas.removeEventListener("keydown", this.keyDown);
    for (const mark of this.marks.values()) { mark.trail.geometry.dispose(); (mark.trail.material as THREE.Material).dispose(); }
    this.marks.clear(); for (const g of this.ownedGeometry) g.dispose(); for (const m of this.ownedMaterials) m.dispose();
    this.scene.clear(); this.renderer.dispose(); this.renderer.forceContextLoss(); canvas.remove();
  }
}
