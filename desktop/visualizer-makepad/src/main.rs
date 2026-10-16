//! CodeCity: optional GPU visualization of Agent eBPF's compact CodeSpace events.
//! Based on the public Makepad Box3D offscreen-pass/instanced-mesh API.
//! This process is read-only and never starts or configures a collector.
mod model;
mod stream;
mod timeline;

pub use makepad_widgets;
pub use makepad_xr;

use makepad_widgets::*;
use makepad_xr::scene::*;
use model::{Activity, City, MAX_EVENTS_PER_FRAME};
use timeline::{Lane, LaneSelection};
use std::path::PathBuf;
use std::sync::atomic::Ordering;
use stream::EventFeed;

app_main!(App);

script_mod! {
    use mod.prelude.widgets.*
    use mod.widgets.*
    use mod.math.*
    use mod.shader.*
    use mod.draw
    use mod.geom

    // Per-instance material: one shared cube mesh, no texture allocations per file.
    mod.draw.DrawCityMesh = mod.std.set_type_default() do #(DrawCityMesh::script_shader(vm)){
        alpha_blend: false
        backface_culling: true
        vertex_pos: vertex_position(vec4f)
        fb0: fragment_output(0, vec4f)
        draw_call: uniform_buffer(draw.DrawCallUniforms)
        draw_pass: uniform_buffer(draw.DrawPassUniforms)
        draw_list: uniform_buffer(draw.DrawListUniforms)
        geom: vertex_buffer(geom.IcoVertex, geom.IcoGeom)
        v_world_clip: varying(vec4f)
        v_normal: varying(vec3f)

        vertex: fn() {
            let local_pos = vec3(
                self.geom.pos.x * self.scale.x,
                self.geom.pos.y * self.scale.y,
                self.geom.pos.z * self.scale.z
            )
            let local_normal = normalize(vec3(
                self.geom.normal.x / max(self.scale.x, 0.00001),
                self.geom.normal.y / max(self.scale.y, 0.00001),
                self.geom.normal.z / max(self.scale.z, 0.00001)
            ))
            let model_view = self.draw_list.view_transform * self.transform
            let world = model_view * vec4(local_pos.x, local_pos.y, local_pos.z, 1.0)
            self.v_normal = normalize((model_view * vec4(local_normal.x, local_normal.y, local_normal.z, 0.0)).xyz)
            self.v_world_clip = vec4(world.x, world.y, world.z, 1.0)
            let view_pos = self.draw_pass.camera_view * world
            self.vertex_pos = self.draw_pass.camera_projection * view_pos
        }

        pixel: fn() -> vec4f {
            let n = normalize(self.v_normal)
            let light = max(dot(n, normalize(vec3(-0.4, 0.8, 0.42))), 0.0)
            let fill = max(dot(n, normalize(vec3(0.6, 0.3, -0.3))), 0.0)
            return vec4(self.color.xyz * (0.32 + 0.58 * light + 0.15 * fill), self.color.w)
        }

        fragment: fn() {
            self.fb0 = depth_clip(self.v_world_clip, self.pixel(), self.depth_clip)
        }
    }

    mod.widgets.CityViewportBase = #(CityViewport::register_widget(vm))
    mod.widgets.CityViewport = set_type_default() do mod.widgets.CityViewportBase{
        width: Fill
        height: Fill
        clear_color: #x0c1120
        draw_bg: mod.draw.DrawXrSceneTexture{}
        draw_mesh: mod.draw.DrawCityMesh{backface_culling: true}
        draw_label: mod.draw.DrawText3d{}
        camera: mod.widgets.XrCamera{
            fov_y: 43.0
            desktop_target: vec3(0.0, 1.0, 0.0)
            distance: 82.0
            distance_min: 8.0
            distance_max: 200.0
            wheel_zoom_step: 0.08
        }
    }

    load_all_resources() do #(App::script_component(vm)){
        ui: Root{
            main_window := Window{
                window.inner_size: vec2(1280, 800)
                body +: {
                    app_view := SolidView{
                        width: Fill
                        height: Fill
                        flow: Down
                        draw_bg +: {color: #x0c1120}
                        header := SolidView{
                            width: Fill
                            height: 56.0
                            flow: Right
                            align: Align{x: 0.0 y: 0.5}
                            padding: Inset{left: 20.0 right: 20.0}
                            spacing: 18.0
                            draw_bg +: {color: #x151c2b}
                            title := H3{
                                text: "明镜高悬  /  CodeCity"
                                draw_text +: {color: #xdfe7fc}
                            }
                            status_hint := Label{
                                text: "● CONNECTING  •  drag: orbit  /  wheel: zoom"
                                draw_text +: {color: #x91a3bf}
                            }
                        }
                        time_toolbar := SolidView{
                            width: Fill
                            height: 44.0
                            flow: Right
                            spacing: 7.0
                            align: Align{x: 0.0 y: 0.5}
                            padding: Inset{left: 12.0 right: 12.0}
                            draw_bg +: {color: #x161c2b}
                            time_title := Label{text: "Time span" draw_text +: {color: #x9eacd1}}
                            time_1 := Button{text: "1m"}
                            time_5 := Button{text: "5m"}
                            time_15 := Button{text: "15m"}
                            time_60 := Button{text: "1h"}
                            time_all := Button{text: "All"}
                            time_shrink := Button{text: "−"}
                            time_grow := Button{text: "+"}
                            time_older := Button{text: "◀ Older"}
                            time_newer := Button{text: "Newer ▶"}
                            time_live := Button{text: "Live"}
                        }
                        actor_toolbar := SolidView{
                            width: Fill
                            height: 44.0
                            flow: Right
                            spacing: 8.0
                            align: Align{x: 0.0 y: 0.5}
                            padding: Inset{left: 12.0 right: 12.0}
                            draw_bg +: {color: #x121a29}
                            actor_all := Button{text: "All actors"}
                            actor_root := Button{text: "Main"}
                            actor_child := Button{text: "Child processes"}
                            actor_verified := Button{text: "Subagents ✓"}
                            actor_prev := Button{text: "◀ Lane"}
                            actor_next := Button{text: "Lane ▶"}
                            actor_info := Label{
                                text: "ALL ACTORS"
                                draw_text +: {color: #xf0c2f2}
                            }
                        }
                        viewport := mod.widgets.CityViewport{}
                        insight := SolidView{
                            width: Fill
                            height: 30.0
                            flow: Right
                            align: Align{x: 0.0 y: 0.5}
                            padding: Inset{left: 20.0 right: 20.0}
                            draw_bg +: {color: #x101827}
                            event_detail := Label{
                                text: "No observed file activity yet"
                                draw_text +: {color: #xd9bbed}
                            }
                        }
                        footer := SolidView{
                            width: Fill
                            height: 44.0
                            flow: Right
                            align: Align{x: 0.0 y: 0.5}
                            padding: Inset{left: 20.0 right: 20.0}
                            spacing: 18.0
                            draw_bg +: {color: #x151c2b}
                            legend := Label{
                                text: "CYAN read  /  TEAL access  /  PINK write  /  AMBER alert  /  RED blocked   •   ←/→ replay   Esc live"
                                draw_text +: {color: #xb0bed6}
                            }
                        }
                    }
                }
            }
        }
    }
}

#[derive(Script, ScriptHook, Debug)]
#[repr(C)]
pub struct DrawCityMesh {
    #[deref]
    draw_vars: DrawVars,
    #[live]
    color: Vec4f,
    #[live]
    transform: Mat4f,
    #[live(vec3(1.0, 1.0, 1.0))]
    scale: Vec3f,
    #[live(1.0)]
    depth_clip: f32,
}

impl DrawCityMesh {
    fn draw(&mut self, cx: &mut CxDraw, geometry_id: GeometryId) {
        self.draw_vars.geometry_id = Some(geometry_id);
        if self.draw_vars.can_instance() {
            let area = cx.add_instance(&self.draw_vars);
            self.draw_vars.area = cx.update_area_refs(self.draw_vars.area, area);
        }
    }

    fn cube(&mut self, cx: &mut CxDraw, id: GeometryId, position: Vec3f, half: Vec3f, color: Vec4f) {
        self.transform = Mat4f::translation(position);
        self.scale = half;
        self.color = color;
        self.depth_clip = 0.0;
        self.draw(cx, id);
    }
}

fn cube_geometry(cx: &mut Cx, slot: &mut Option<Geometry>) -> GeometryId {
    let geometry = slot.get_or_insert_with(|| {
        let geometry = Geometry::new(cx);
        let faces: [([f32;3], [f32;3], [f32;3]); 6] = [
            ([1.,0.,0.], [0.,1.,0.], [0.,0.,1.]),
            ([-1.,0.,0.], [0.,0.,1.], [0.,1.,0.]),
            ([0.,1.,0.], [0.,0.,1.], [1.,0.,0.]),
            ([0.,-1.,0.], [1.,0.,0.], [0.,0.,1.]),
            ([0.,0.,1.], [1.,0.,0.], [0.,1.,0.]),
            ([0.,0.,-1.], [0.,1.,0.], [1.,0.,0.]),
        ];
        let mut vertices = Vec::with_capacity(6*4*8);
        let mut indices = Vec::with_capacity(36);
        for (n,u,v) in faces {
            let base = (vertices.len()/8) as u32;
            for (su, sv) in [(-1.0f32,-1.0f32),(1.,-1.),(1.,1.),(-1.,1.)] {
                let p = [n[0]+u[0]*su+v[0]*sv, n[1]+u[1]*su+v[1]*sv, n[2]+u[2]*su+v[2]*sv];
                vertices.extend_from_slice(&[p[0],p[1],p[2],1., n[0],n[1],n[2],0.]);
            }
            indices.extend_from_slice(&[base,base+1,base+2,base,base+2,base+3]);
        }
        geometry.update(cx, indices, vertices);
        geometry
    });
    geometry.geometry_id()
}

fn material(activity: Activity, pulse: f32, agent: bool) -> Vec4f {
    let base = match activity {
        Activity::Read    => vec3(0.08, 0.78, 0.98),
        Activity::Write   => vec3(0.98, 0.33, 0.78),
        Activity::Blocked => vec3(0.98, 0.15, 0.27),
        Activity::Alert   => vec3(1.0, 0.62, 0.23),
        Activity::Access  => vec3(0.19, 0.73, 0.67),
    };
    let intensity = if agent {0.63} else {0.38} + pulse * 0.37;
    vec4((base.x*intensity).min(1.0), (base.y*intensity).min(1.0),
         (base.z*intensity).min(1.0), 1.0)
}

#[derive(Script, ScriptHook, WidgetRef, WidgetRegister)]
pub struct CityViewport {
    #[uid]
    uid: WidgetUid,
    #[source]
    source: ScriptObjectRef,
    #[walk]
    walk: Walk,
    #[layout]
    layout: Layout,
    #[live]
    draw_bg: DrawXrSceneTexture,
    #[live]
    draw_mesh: DrawCityMesh,
    #[live]
    draw_label: DrawText3d,
    #[live(vec4(0.047, 0.067, 0.125, 1.0))]
    clear_color: Vec4f,
    #[live]
    camera: XrCamera,
    #[new]
    pass: DrawPass,
    #[new]
    draw_list: DrawList,
    #[new]
    color_texture: Texture,
    #[new]
    depth_texture: Texture,
    #[rust]
    area: Area,
    #[rust(false)]
    initialized: bool,
    #[rust]
    geometry: Option<Geometry>,
    #[rust]
    city: City,
    #[rust]
    selected_file: Option<usize>,
    #[rust]
    pick_points: Vec<(f32, f32, f32, usize)>,
    #[rust]
    workspace_root: Option<PathBuf>,
    #[rust]
    feed: Option<EventFeed>,
    #[rust]
    next_frame: NextFrame,
}

impl CityViewport {
    fn initialize(&mut self, cx: &mut Cx) {
        if self.initialized { return; }
        self.initialized = true;
        self.camera.orbit_yaw = 0.72;
        self.camera.orbit_pitch = -0.60;
        self.color_texture = Texture::new_with_format(cx, TextureFormat::RenderBGRAu8 {
            size: TextureSize::Auto, initial: true,
        });
        self.depth_texture = Texture::new_with_format(cx, TextureFormat::DepthD32 {
            size: TextureSize::Auto, initial: true,
        });
        self.pass.set_color_texture(cx, &self.color_texture,
            DrawPassClearColor::ClearWith(self.clear_color));
        self.pass.set_depth_texture(cx, &self.depth_texture,
            DrawPassClearDepth::ClearWith(1.0));
        cx.passes[self.pass.draw_pass_id()].keep_camera_matrix = true;
        self.workspace_root = model::workspace_root();
        let run_id = std::env::var("AGENT_EBPF_AGENT_RUN_ID").ok();
        let root_pid = std::env::var("AGENT_EBPF_AGENT_ROOT_PID").ok()
            .and_then(|pid| pid.parse::<i32>().ok());
        let agent_only = std::env::var("AGENT_EBPF_AGENT_ONLY")
            .ok().is_some_and(|value| value == "1" || value.eq_ignore_ascii_case("true"));
        self.city.set_agent_scope(run_id, root_pid, agent_only);
        // An optional remote CodeSpace inventory is names-only JSON. The file
        // itself must be regular, non-symlink, and small; never read source.
        if let Some(index) = std::env::var_os("AGENT_EBPF_INDEX_JSON") {
            let path = std::path::PathBuf::from(index);
            if std::fs::symlink_metadata(&path).ok()
                .is_some_and(|meta| meta.is_file()
                    && !meta.file_type().is_symlink() && meta.len() <= 1024 * 1024)
            {
                if let Ok(bytes) = std::fs::read(path) {
                    let _ = self.city.seed_manifest(&bytes);
                }
            }
        }
        if let Some(root) = self.workspace_root.as_deref() {
            if model::local_scan_root(root) {
                self.city.seed_workspace(root);
            }
        }
        self.feed = Some(stream::subscribe());
        self.next_frame = cx.new_next_frame();
    }

    fn pull_feed(&mut self) {
        if let Some(feed) = &self.feed {
            // Health and lost-message counters cannot be starved by a full
            // event queue (e.g. when switching away from the desktop window).
            self.city.connected = feed.online.load(Ordering::Acquire);
            self.city.transport_dropped = feed.dropped.load(Ordering::Relaxed);
            for _ in 0..MAX_EVENTS_PER_FRAME {
                match feed.events.try_recv() {
                    Ok(event) => { let _ = self.city.apply(event, self.workspace_root.as_deref()); }
                    Err(std::sync::mpsc::TryRecvError::Empty) => break,
                    Err(std::sync::mpsc::TryRecvError::Disconnected) => {
                        self.city.connected = false;
                        break;
                    }
                }
            }
        }
    }

    fn selected_note(&self) -> String {
        if let Some(i) = self.selected_file {
            if let Some(b) = self.city.buildings.get(i) {
                let path = &b.path;
                let path = if path.chars().count() > 100 {
                    let pos = path.char_indices()
                        .nth(path.chars().count() - 99)
                        .map(|(i, _)| i).unwrap_or(0);
                    format!("…{}", &path[pos..])
                } else { path.to_owned() };
                let kind = match b.activity {
                    Activity::Read => "READ",
                    Activity::Write => "WRITE",
                    Activity::Alert => "ALERT",
                    Activity::Blocked => "BLOCKED",
                    Activity::Access => "ACCESS",
                };
                return format!("SELECTED • {kind} • {path} • {}",
                    if b.owned_by_agent { "AGENT" } else { "UNATTRIBUTED" });
            }
        }
        self.city.latest_note()
    }

    fn draw_city(&mut self, cx: &mut Cx3d, state: SceneState3D) {
        self.draw_list.begin_always(cx);
        cx.begin_scene_3d(state);
        let previous = cx.set_scene_world_transform_3d(Mat4f::identity());
        let cube = cube_geometry(cx.cx, &mut self.geometry);

        // Stage: flat grid, scene-light reference, connection beacon.
        self.draw_mesh.cube(cx, cube, vec3(0., -0.18, 0.), vec3(32.,0.12,28.),
            vec4(0.09,0.12,0.21,1.));
        let beacon = if self.city.connected {
            vec4(0.19,0.95,0.55,1.)
        } else {
            vec4(1.0,0.58,0.14,1.)
        };
        self.draw_mesh.cube(cx, cube, vec3(-29., 2.0, -25.),
            vec3(0.3,2.0,0.3), beacon);

        for (i, building) in self.city.buildings.iter().enumerate() {
            let h = building.height;
            // An inactive filename still remains in the topological city,
            // but cannot masquerade as an event within the chosen time span.
            if !building.visible {
                self.draw_mesh.cube(cx, cube, vec3(building.x, h, building.z),
                    vec3(0.51, h, 0.51), vec4(0.09, 0.12, 0.20, 1.0));
                continue;
            }
            // Actor-local accents are intentionally separate from the semantic
            // file-operation color of the building and signal column.
            let actor_tint = match building.last_lane.as_ref() {
                Some(Lane::VerifiedSubagent { .. }) => vec4(0.74,0.45,0.97,1.0),
                Some(Lane::Delegated { .. }) => vec4(0.98,0.64,0.26,1.0),
                Some(Lane::Root { .. }) => vec4(0.18,0.90,0.96,1.0),
                _ => vec4(0.33,0.36,0.48,1.0),
            };
            self.draw_mesh.cube(cx, cube, vec3(building.x,-0.012,building.z),
                vec3(0.57,0.04,0.57), actor_tint);
            if self.selected_file == Some(i) {
                // A slightly larger silver shell under the selected tower.
                self.draw_mesh.cube(cx, cube, vec3(building.x, h, building.z),
                    vec3(0.58, h + 0.07, 0.58), vec4(0.83, 0.86, 1.0, 1.0));
            }
            let color = material(building.activity, building.pulse, building.owned_by_agent);
            self.draw_mesh.cube(cx, cube, vec3(building.x, h, building.z),
                vec3(0.51, h, 0.51), color);
            if building.pulse > 0.01 {
                // Instanced signal beam: taller for fresh events, decays to zero.
                self.draw_mesh.cube(cx, cube,
                    vec3(building.x, 2.0 + 5.0 * building.pulse, building.z),
                    vec3(0.07, 2.0 + 5.0 * building.pulse, 0.07),
                    material(building.activity, 1.0, true));
            }
        }
        if let Some(previous) = previous {
            let _ = cx.set_scene_world_transform_3d(previous);
        }
        cx.end_scene_3d();
        self.draw_list.end(cx);
    }
}

fn pass_camera(cx: &mut Cx, pass: &DrawPass, state: &SceneState3D) {
    let u = &mut cx.passes[pass.draw_pass_id()].pass_uniforms;
    u.camera_projection = state.projection;
    u.camera_projection_r = state.projection;
    u.camera_view = state.view;
    u.camera_view_r = state.view;
    u.depth_projection = state.projection;
    u.depth_projection_r = state.projection;
    u.depth_view = state.view;
    u.depth_view_r = state.view;
    u.camera_inv = state.view.invert();
    u.camera_inv_r = state.view.invert();
}

impl WidgetNode for CityViewport {
    fn widget_uid(&self) -> WidgetUid { self.uid }
    fn walk(&mut self, _cx: &mut Cx) -> Walk { self.walk }
    fn area(&self) -> Area { self.area }
    fn redraw(&mut self, cx: &mut Cx) { self.area.redraw(cx); }
}

impl Widget for CityViewport {
    fn handle_event(&mut self, cx: &mut Cx, event: &Event, _scope: &mut Scope) {
        self.camera.handle_desktop_interaction(cx, event);
        // Click/tap selects the nearest projected tower. Camera dragging is
        // not a tap and therefore never accidentally selects a building.
        if let Hit::FingerUp(hit) = event.hits(cx, self.area) {
            if hit.is_primary_hit() && hit.was_tap() {
                let x = hit.abs.x as f32;
                let y = hit.abs.y as f32;
                self.selected_file = self.pick_points.iter()
                    .filter_map(|(bx, by, depth, i)| {
                        let distance = (x - *bx).powi(2) + (y - *by).powi(2);
                        (distance <= 24.0 * 24.0).then_some((distance, *depth, *i))
                    })
                    .min_by(|a, b| a.0.total_cmp(&b.0).then(a.1.total_cmp(&b.1)))
                    .map(|(_, _, i)| i);
                self.area.redraw(cx);
            }
        }
        if let Event::KeyDown(key) = event {
            match key.key_code {
                KeyCode::ArrowLeft => self.city.replay_back(),
                KeyCode::ArrowRight => self.city.replay_forward(),
                KeyCode::Escape => self.city.resume_live(),
                _ => {}
            }
        }
        if self.next_frame.is_event(event).is_some() {
            self.pull_feed();
            self.city.step();
            self.next_frame = cx.new_next_frame();
            self.area.redraw(cx);
        }
    }

    fn draw_walk(&mut self, cx: &mut Cx2d, _scope: &mut Scope, walk: Walk) -> DrawStep {
        let rect = cx.walk_turtle_with_area(&mut self.area, walk);
        if rect.size.x <= 1.0 || rect.size.y <= 1.0 { return DrawStep::done(); }
        self.initialize(cx.cx);
        self.camera.set_desktop_viewport_rect(rect);
        self.pass.set_size(cx, rect.size);
        self.pass.set_color_texture(cx, &self.color_texture,
            DrawPassClearColor::ClearWith(self.clear_color));
        self.pass.set_depth_texture(cx, &self.depth_texture,
            DrawPassClearDepth::ClearWith(1.0));

        cx.make_child_pass(&self.pass);
        cx.begin_pass(&self.pass, None);
        if let Some(state) = self.camera.desktop_scene_state(rect, cx.time()) {
            pass_camera(cx.cx, &self.pass, &state);
            let cx3d = &mut Cx3d::new(cx.cx);
            self.draw_city(cx3d, state);
        }
        cx.end_pass(&self.pass);
        self.draw_bg.set_scene_texture(&self.color_texture);
        self.draw_bg.draw_abs(cx, rect);
        // A small number of directory headings is projected into native 2D
        // overlay text. This avoids creating per-file 3D text meshes.
        if let Some(state) = self.camera.desktop_scene_state(rect, cx.time()) {
            let camera = state.view.invert().transform_vec4(vec4(0.0, 0.0, 0.0, 1.0));
            let inv = 1.0 / camera.w.max(0.00001);
            self.draw_label.set_camera_state(state.view, state.projection,
                vec3(camera.x * inv, camera.y * inv, camera.z * inv));
            self.draw_label.set_viewport_rect(rect);
            self.pick_points.clear();
            // The GPU scene is authoritative; project building rooftops using
            // the same camera as the labels for read-only mouse picking.
            for (i, b) in self.city.buildings.iter().enumerate() {
                if !b.visible { continue; }
                if let Some((screen, depth)) = self.draw_label
                    .project_world_to_screen(vec3(b.x, b.height * 2.0, b.z))
                {
                    if screen.x >= rect.pos.x as f32 && screen.y >= rect.pos.y as f32
                        && screen.x <= (rect.pos.x + rect.size.x) as f32
                        && screen.y <= (rect.pos.y + rect.size.y) as f32 {
                        self.pick_points.push((screen.x, screen.y, depth, i));
                    }
                }
            }
            for (name, x, z) in self.city.district_labels(18) {
                let _ = self.draw_label.draw_world_text(cx, vec3(x, 5.0, z), &name);
            }
            // Recent events float above their own towers, like commit lights.
            let mut shown = 0;
            for b in self.city.buildings.iter().rev() {
                if !b.visible || b.pulse < 0.88 { continue; }
                let label = b.path.rsplit('/').next().unwrap_or(&b.path);
                if shown >= 8 { break; }
                let _ = self.draw_label.draw_world_text(cx,
                    vec3(b.x, b.height * 2.0 + 2.0, b.z), label);
                shown += 1;
            }
        }
        self.area = self.draw_bg.area();
        cx.set_pass_area(&self.pass, self.area);
        DrawStep::done()
    }
}

#[derive(Script, ScriptHook)]
pub struct App {
    #[live]
    ui: WidgetRef,
    #[rust]
    last_status: String,
    #[rust]
    last_note: String,
    #[rust]
    last_actor: String,
}

#[derive(Clone, Copy)]
enum ToolbarCommand {
    Span(u32), Shrink, Grow, Older, Newer, Live,
    AllActors, MainActors, ChildProcesses, VerifiedSubagents,
    PreviousActor, NextActor,
}

impl MatchEvent for App {
    fn handle_actions(&mut self, cx: &mut Cx, actions: &Actions) {
        let command = if self.ui.button(cx, ids!(time_1)).clicked(actions) {
            Some(ToolbarCommand::Span(1))
        } else if self.ui.button(cx, ids!(time_5)).clicked(actions) {
            Some(ToolbarCommand::Span(5))
        } else if self.ui.button(cx, ids!(time_15)).clicked(actions) {
            Some(ToolbarCommand::Span(15))
        } else if self.ui.button(cx, ids!(time_60)).clicked(actions) {
            Some(ToolbarCommand::Span(60))
        } else if self.ui.button(cx, ids!(time_all)).clicked(actions) {
            Some(ToolbarCommand::Span(0))
        } else if self.ui.button(cx, ids!(time_shrink)).clicked(actions) {
            Some(ToolbarCommand::Shrink)
        } else if self.ui.button(cx, ids!(time_grow)).clicked(actions) {
            Some(ToolbarCommand::Grow)
        } else if self.ui.button(cx, ids!(time_older)).clicked(actions) {
            Some(ToolbarCommand::Older)
        } else if self.ui.button(cx, ids!(time_newer)).clicked(actions) {
            Some(ToolbarCommand::Newer)
        } else if self.ui.button(cx, ids!(time_live)).clicked(actions) {
            Some(ToolbarCommand::Live)
        } else if self.ui.button(cx, ids!(actor_all)).clicked(actions) {
            Some(ToolbarCommand::AllActors)
        } else if self.ui.button(cx, ids!(actor_root)).clicked(actions) {
            Some(ToolbarCommand::MainActors)
        } else if self.ui.button(cx, ids!(actor_child)).clicked(actions) {
            Some(ToolbarCommand::ChildProcesses)
        } else if self.ui.button(cx, ids!(actor_verified)).clicked(actions) {
            Some(ToolbarCommand::VerifiedSubagents)
        } else if self.ui.button(cx, ids!(actor_prev)).clicked(actions) {
            Some(ToolbarCommand::PreviousActor)
        } else if self.ui.button(cx, ids!(actor_next)).clicked(actions) {
            Some(ToolbarCommand::NextActor)
        } else { None };
        let Some(command) = command else { return };
        let viewport = self.ui.widget(cx, ids!(viewport));
        if let Some(mut viewport) = viewport.borrow_mut::<CityViewport>() {
            match command {
                ToolbarCommand::Span(minutes) => viewport.city.set_time_span(minutes),
                ToolbarCommand::Shrink => viewport.city.resize_time_span(false),
                ToolbarCommand::Grow => viewport.city.resize_time_span(true),
                ToolbarCommand::Older => viewport.city.shift_time_range(true),
                ToolbarCommand::Newer => viewport.city.shift_time_range(false),
                ToolbarCommand::Live => viewport.city.resume_time_live(),
                ToolbarCommand::AllActors => viewport.city.set_lane_selection(LaneSelection::All),
                ToolbarCommand::MainActors => viewport.city.set_lane_selection(LaneSelection::Main),
                ToolbarCommand::ChildProcesses => viewport.city.set_lane_selection(LaneSelection::Delegated),
                ToolbarCommand::VerifiedSubagents => viewport.city.set_lane_selection(LaneSelection::Verified),
                ToolbarCommand::PreviousActor => viewport.city.cycle_actor(false),
                ToolbarCommand::NextActor => viewport.city.cycle_actor(true),
            };
            viewport.area.redraw(cx);
        };
    }
}

impl AppMain for App {
    fn script_mod(vm: &mut ScriptVm) -> ScriptValue {
        crate::makepad_widgets::script_mod(vm);
        crate::makepad_xr::script_mod(vm);
        self::script_mod(vm)
    }
    fn handle_event(&mut self, cx: &mut Cx, event: &Event) {
        self.match_event(cx, event);
        self.ui.handle_event(cx, event, &mut Scope::empty());
        // Compose lightweight native HUD from the same bounded renderer model.
        // Updating only on change avoids forcing layout work at every GPU tick.
        let stats = self.ui.widget(cx, ids!(viewport))
            .borrow::<CityViewport>()
             .map(|viewport| (viewport.city.status_line(), viewport.selected_note(),
                viewport.city.lane_selection.label()));
        if let Some((status, note, actor)) = stats {
            if status != self.last_status {
                self.ui.label(cx, ids!(status_hint)).set_text(cx, &status);
                self.last_status = status;
            }
            if note != self.last_note {
                self.ui.label(cx, ids!(event_detail)).set_text(cx, &note);
                self.last_note = note;
            }
            if actor != self.last_actor {
                self.ui.label(cx, ids!(actor_info)).set_text(cx, &actor);
                self.last_actor = actor;
            }
        }
    }
}
