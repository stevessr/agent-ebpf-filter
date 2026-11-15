// Command visualgif renders reproducible Agent eBPF architecture animations with
// MyGo's native, headless ui.Render. No window, browser, captured credentials, or
// backend connection is needed: scenes are explicitly illustrative.
package main

import (
 "errors"
 "flag"
 "fmt"
 "image"
 "image/color/palette"
 "image/draw"
 "image/gif"
 "math"
 "os"
 "path/filepath"
 "strings"

 "github.com/egoist/mygo/ui"
)

const (
 canvasWidth = 1280
 canvasHeight = 720
 maxFrames = 300
)

var scenes = map[string]bool{
 "boundary": true,
 "observability": true,
 "decision": true,
}

func main() {
 if err := run(os.Args[1:]); err != nil {
  fmt.Fprintln(os.Stderr, "visualgif:", err)
  os.Exit(1)
 }
}

func run(args []string) error {
 flags := flag.NewFlagSet("visualgif", flag.ContinueOnError)
 scene := flags.String("scene", "boundary", "boundary | observability | decision")
 out := flags.String("out", "", "output GIF path (required)")
 fps := flags.Int("fps", 12, "frames per second (1-30)")
 seconds := flags.Int("seconds", 6, "loop duration in seconds (1-20)")
 if err := flags.Parse(args); err != nil { return err }
 if !scenes[*scene] { return fmt.Errorf("unknown scene %q; choose boundary, observability or decision", *scene) }
 if strings.TrimSpace(*out) == "" { return errors.New("-out is required") }
 if *fps < 1 || *fps > 30 || *seconds < 1 || *seconds > 20 || *fps * *seconds > maxFrames {
  return fmt.Errorf("invalid fps/seconds: at most %d frames, fps 1..30, duration 1..20s", maxFrames)
 }
 if len(flags.Args()) != 0 { return fmt.Errorf("unexpected arguments: %v", flags.Args()) }
 return writeGIF(*out, *scene, *fps, *seconds)
}

func writeGIF(out, scene string, fps, seconds int) error {
 count := fps * seconds
 animation := &gif.GIF{Image: make([]*image.Paletted, 0, count), Delay: make([]int, 0, count), LoopCount: 0}
 for i := 0; i < count; i++ {
  // Time comes from the frame index, not wall-clock time; exports are repeatable.
  phase := float32(i) / float32(count)
  rgba := renderFrame(scene, phase)
  paletted := image.NewPaletted(rgba.Bounds(), palette.Plan9)
  draw.FloydSteinberg.Draw(paletted, paletted.Bounds(), rgba, image.Point{})
  animation.Image = append(animation.Image, paletted)
  // GIF delays are centiseconds. Distribute fractional delays so the total
  // playback duration remains exactly the requested number of seconds.
  current := int(math.Round(float64((i+1)*100) / float64(fps)))
  previous := int(math.Round(float64(i*100) / float64(fps)))
  animation.Delay = append(animation.Delay, current-previous)
 }
 dir := filepath.Dir(out)
 if err := os.MkdirAll(dir, 0755); err != nil { return err }
 file, err := os.CreateTemp(dir, ".visualgif-*.gif")
 if err != nil { return err }
 tmp := file.Name()
 defer os.Remove(tmp)
 if err := gif.EncodeAll(file, animation); err != nil { file.Close(); return err }
 if err := file.Close(); err != nil { return err }
 return os.Rename(tmp, out)
}

var (
 bg = ui.Hex("#091324")
 panel = ui.Hex("#14233A")
 border = ui.Hex("#314865")
 white = ui.Hex("#EFF5FF")
 muted = ui.Hex("#9DB1C9")
 teal = ui.Hex("#38D8C3")
 blue = ui.Hex("#69A8FF")
 amber = ui.Hex("#FFC16A")
 danger = ui.Hex("#FF667E")
)

func renderFrame(scene string, progress float32) *image.RGBA {
 return ui.Render(func(c *ui.Context) {
  c.Root().Background(bg)
  ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
   drawScene(p, r, scene, clamp(progress))
  })
 }, canvasWidth, canvasHeight, 1)
}

func clamp(v float32) float32 {
 if v < 0 { return 0 }
 if v > 1 { return 1 }
 return v
}

func ease(v float32) float32 {
 v = clamp(v)
 return v*v*(3-2*v)
}

func segment(v, start, end float32) float32 {
 return ease((v-start)/(end-start))
}

func drawScene(p *ui.Painter, r ui.Rect, scene string, progress float32) {
 p.Fill(r, bg, 0)
 for x := float32(40); x <= canvasWidth; x += 80 {
  p.StrokePath((&ui.Path{}).MoveTo(x, 0).LineTo(x, canvasHeight), 1, border.Alpha(0.14))
 }
 for y := float32(40); y <= canvasHeight; y += 80 {
  p.StrokePath((&ui.Path{}).MoveTo(0, y).LineTo(canvasWidth, y), 1, border.Alpha(0.14))
 }
 p.Fill(ui.Rect{X: 58, Y: 42, W: 8, H: 54}, teal, 4)
 p.Text(85, 39, "AGENT eBPF FILTER", 32, white)
 p.Text(85, 87, "Native MyGo Render  /  Security & Observability", 16, muted)
 p.Fill(ui.Rect{X: 1030, Y: 51, W: 194, H: 34}, panel, 12)
 p.Stroke(ui.Rect{X: 1030, Y: 51, W: 194, H: 34}, border, 12, 1)
 p.Text(1044, 58, "ILLUSTRATIVE DEMO", 12, amber)

 switch scene {
 case "boundary": paintBoundary(p, progress)
 case "observability": paintObservability(p, progress)
 case "decision": paintDecision(p, progress)
 }
 p.Text(65, 644, "SIMULATED SEQUENCE  |  Not evidence of detected attacks or measured protection", 13, muted)
 p.Fill(ui.Rect{X: 64, Y: 684, W: 1152, H: 4}, border, 2)
 p.Fill(ui.Rect{X: 64, Y: 684, W: 1152*progress, H: 4}, teal, 2)
}

func card(p *ui.Painter, x,y,w,h float32, label, detail string, tone ui.Color, active float32) {
 bounds := ui.Rect{X:x,Y:y,W:w,H:h}
 p.Fill(bounds, panel, 18)
 p.Stroke(bounds, border, 18, 1.5)
 p.Fill(ui.Rect{X:x+18,Y:y+18,W:6,H:h-36}, tone.Alpha(0.55+0.45*active), 3)
 p.Text(x+38, y+25, label, 21, white)
 p.Text(x+38, y+68, detail, 14, muted)
 if active > 0 {
  p.Stroke(bounds, tone.Alpha(active), 18, 2.8)
  pulse := float32(math.Mod(float64(active*1.7), 1))
  p.FillPath((&ui.Path{}).Circle(x+w-23, y+26, 5+8*pulse), tone.Alpha((1-pulse)*0.25))
  p.FillPath((&ui.Path{}).Circle(x+w-23, y+26, 5), tone)
 }
}

func arrow(p *ui.Painter, x1,y1,x2,y2 float32, tone ui.Color, active float32) {
 p.StrokePath((&ui.Path{}).MoveTo(x1,y1).LineTo(x2,y2), 3, border)
 if active > 0 {
  p.StrokePath((&ui.Path{}).MoveTo(x1,y1).LineTo(x1+(x2-x1)*active,y1+(y2-y1)*active), 4, tone)
  x,y := x1+(x2-x1)*active,y1+(y2-y1)*active
  p.FillPath((&ui.Path{}).Circle(x,y,7),tone)
 }
 a := (&ui.Path{}).MoveTo(x2-10,y2-7).LineTo(x2,y2).LineTo(x2-10,y2+7).Close()
 p.FillPath(a, tone.Alpha(0.6))
}

func chip(p *ui.Painter, x,y,w float32, label string, tone ui.Color, on float32) {
 rect:=ui.Rect{X:x,Y:y,W:w,H:34}
 p.Fill(rect, tone.Alpha(0.09+on*0.19), 10)
 p.Stroke(rect,tone.Alpha(0.45+on*0.55),10,1.3)
 p.Text(x+13,y+8,label,13,tone)
}

func paintBoundary(p *ui.Painter, v float32) {
 p.Text(65, 149, "01   The semantic gap: intent is not kernel fact", 22, white)
 p.Text(65, 184, "Follow the action across the trust boundary. Observe OS facts before policy enforcement.", 15, muted)
 xs := []float32{65, 355, 645, 935}
 titles:=[]string{"LLM / Agent", "OS syscall", "eBPF probe", "Policy engine"}
 notes:=[]string{"Untrusted intent", "openat / connect", "Tracepoint / LSM", "Allow or deny"}
 tones:=[]ui.Color{blue, amber, teal, danger}
 for i:=range xs {
  active:=segment(v,float32(i)*0.19,float32(i)*0.19+0.23)
  card(p,xs[i],260,235,152,titles[i],notes[i],tones[i],active)
  if i<3 { arrow(p,xs[i]+239,336,xs[i+1]-8,336,tones[i+1],segment(v,float32(i)*0.23+0.12,float32(i)*0.23+0.38)) }
 }
 chip(p,72,454,254,"INTENT: read a config file",blue,segment(v,0,0.25))
 chip(p,357,454,245,"FACT: openat(...) observed",amber,segment(v,0.2,0.53))
 chip(p,646,454,246,"EVIDENCE: syscall record",teal,segment(v,0.47,0.78))
 if v>0.69 { chip(p,941,454,239,"LSM: access denied",danger,segment(v,0.69,0.91)) }
 p.Text(72,552,"Kernel events reveal the attempted operation; they do not independently prove the prompt's intent.",15,muted)
}

func paintObservability(p *ui.Painter, v float32) {
 p.Text(65,149,"02   Provenance: correlate descendants and kernel events",22,white)
 p.Text(65,184,"An observed process tree plus system events; logical SubAgents need their own verified IDs.",15,muted)
 panelA:=ui.Rect{X:65,Y:224,W:1150,H:364}
 p.Fill(panelA,panel,20)
 p.Stroke(panelA,border,20,1.6)
 type node struct { x,y float32; name,details string; color ui.Color; parent int }
 nodes:=[]node{
  {560,293,"Agent (PID 100)","Observed root",blue,-1},
  {295,402,"bash (PID 110)","Child process",teal,0},
  {610,414,"python (PID 120)","Child process",teal,0},
  {953,402,"curl (PID 130)","Network attempt",amber,0},
  {610,536,"openat(...)","File access",danger,2},
 }
 for i,n:=range nodes {
  if n.parent >= 0 {
   from:=nodes[n.parent]
   arrow(p,from.x,from.y+17,n.x,n.y-32,n.color,segment(v,0.12+float32(i)*0.115,0.29+float32(i)*0.115))
  }
 }
 for i,n:=range nodes {
  active:=segment(v,float32(i)*0.12,float32(i)*0.12+0.22)
  radius:=float32(20)
  if i==0 { radius=27 }
  p.FillPath((&ui.Path{}).Circle(n.x,n.y,radius+6),n.color.Alpha(0.08+0.17*active))
  p.FillPath((&ui.Path{}).Circle(n.x,n.y,radius),n.color)
  p.FillPath((&ui.Path{}).Circle(n.x,n.y,radius*0.45),bg)
  p.Text(n.x-64,n.y+30,n.name,14,white)
  p.Text(n.x-64,n.y+48,n.details,11,muted)
 }
 chip(p,90,245,211,"PARENT / CHILD",blue,1)
 chip(p,920,245,265,"NETWORK + FILE EVENTS",teal,segment(v,0.32,0.7))
}

func paintDecision(p *ui.Painter, v float32) {
 p.Text(65,149,"03   From evidence to a policy verdict",22,white)
 p.Text(65,184,"Cross-layer correlation matters; blocking requires an actual configured and attached enforcement hook.",15,muted)
 stages:=[]struct{x float32; title,detail string; col ui.Color}{
  {70,"Prompt context","Possible injection",blue},
  {475,"Kernel evidence","openat('/etc/shadow')",amber},
  {880,"Policy gate","BPF LSM check",teal},
 }
 for i,s:=range stages {
  card(p,s.x,250,320,147,s.title,s.detail,s.col,segment(v,float32(i)*0.23,float32(i)*0.23+0.28))
  if i<2 { arrow(p,s.x+325,320,stages[i+1].x-8,320,stages[i+1].col,segment(v,float32(i)*0.24+0.12,float32(i)*0.24+0.39)) }
 }
 verdict := segment(v,0.65,0.91)
 p.Fill(ui.Rect{X:360,Y:463,W:560,H:118},panel,20)
 p.Stroke(ui.Rect{X:360,Y:463,W:560,H:118},danger.Alpha(0.25+0.75*verdict),20,2.7)
 p.Fill(ui.Rect{X:385,Y:487,W:12,H:70},danger.Alpha(0.25+0.75*verdict),6)
 if verdict < 0.2 {
  p.Text(418,482,"EVALUATING OBSERVED ACTION",24,white)
  p.Text(420,528,"Awaiting configured enforcement decision",15,muted)
 } else {
  p.Text(418,482,"DENIED  /  BPF LSM",27,danger)
  p.Text(420,528,"Illustrative blocked file-access attempt",15,muted)
 }
}
