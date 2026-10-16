//! CodeCity: optional GPU visualization of Agent eBPF's compact CodeSpace events.
//! Based on the public Makepad Box3D offscreen-pass/instanced-mesh API.
//! This process is read-only and never starts or configures a collector.
mod model;
mod stream;

pub use makepad_widgets;
pub use makepad_xr;

use makepad_widgets::*;
use makepad_xr::scene::*;
use model::{Activity, City, MAX_EVENTS_PER_FRAME};
use std::path::PathBuf;
use std::sync::mpsc::Receiver;
use stream::FeedMessage;

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
                                text: "LIVE FILE ACTIVITY  •  drag: orbit  /  wheel: zoom"
                                draw_text +: {color: #x91a3bf}
                            }
                        }
                        viewport := mod.widgets.CityViewport{}
                        footer := SolidView{
                            width: Fill
                            height: 44.0
                            flow: Right
                            align: Align{x: 0.0 y: 0.5}
                            padding: Inset{left: 20.0 right: 20.0}
                            spacing: 18.0
                            draw_bg +: {color: #x151c2b}
                            legend := Label{
                                text: "CYAN read   /   TEAL access   /   PINK write   /   AMBER alert   /   RED blocked   •   beacon: GREEN connected, AMBER offline"
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
    workspace_root: Option<PathBuf>,
    #[rust]
    feed: Option<Receiver<FeedMessage>>,
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
        if let Some(root) = self.workspace_root.as_deref() {
            self.city.seed_workspace(root);
        }
        self.feed = Some(stream::subscribe());
        self.next_frame = cx.new_next_frame();
    }

    fn pull_feed(&mut self) {
        if let Some(receiver) = &self.feed {
            for _ in 0..MAX_EVENTS_PER_FRAME {
                match receiver.try_recv() {
                    Ok(FeedMessage::Connected(connected)) => self.city.connected = connected,
                    Ok(FeedMessage::Event(event)) => {
                        let _ = self.city.apply(event, self.workspace_root.as_deref());
                    }
                    Err(std::sync::mpsc::TryRecvError::Empty) => break,
                    Err(std::sync::mpsc::TryRecvError::Disconnected) => {
                        self.city.connected = false;
                        break;
                    }
                }
            }
        }
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

        for building in &self.city.buildings {
            let h = building.height;
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
        self.area = self.draw_bg.area();
        cx.set_pass_area(&self.pass, self.area);
        DrawStep::done()
    }
}

#[derive(Script, ScriptHook)]
pub struct App { #[live] ui: WidgetRef }

impl MatchEvent for App {}

impl AppMain for App {
    fn script_mod(vm: &mut ScriptVm) -> ScriptValue {
        crate::makepad_widgets::script_mod(vm);
        crate::makepad_xr::script_mod(vm);
        self::script_mod(vm)
    }
    fn handle_event(&mut self, cx: &mut Cx, event: &Event) {
        self.match_event(cx, event);
        self.ui.handle_event(cx, event, &mut Scope::empty());
    }
}
