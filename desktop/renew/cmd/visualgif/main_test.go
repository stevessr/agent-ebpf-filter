package main

import (
 "bytes"
 "image/gif"
 "os"
 "path/filepath"
 "testing"
)

func TestScenesRenderDifferentFrames(t *testing.T) {
 for scene := range scenes {
  first := renderFrame(scene, 0.05)
  last := renderFrame(scene, 0.95)
  if first.Bounds().Dx() != canvasWidth || first.Bounds().Dy() != canvasHeight {
   t.Fatalf("%s: unexpected bounds %v",scene,first.Bounds())
  }
  if bytes.Equal(first.Pix,last.Pix) { t.Errorf("%s: animation is static",scene) }
 }
}

func TestGIFExportAndInvalidArguments(t *testing.T) {
 if err:=run([]string{"-scene","unknown","-out","x.gif"});err==nil {t.Fatal("unknown scene must fail")}
 if err:=run([]string{"-scene","boundary"});err==nil {t.Fatal("missing output must fail")}
 if err:=run([]string{"-scene","decision","-out","x.gif","-fps","31"});err==nil {t.Fatal("invalid FPS must fail")}
 out:=filepath.Join(t.TempDir(),"sample.gif")
 if err:=run([]string{"-scene","boundary","-fps","2","-seconds","2","-out",out});err!=nil {t.Fatal(err)}
 blob,err:=os.ReadFile(out)
 if err!=nil {t.Fatal(err)}
 animation,err:=gif.DecodeAll(bytes.NewReader(blob))
 if err!=nil {t.Fatal(err)}
 if len(animation.Image)!=4 || animation.Config.Width!=canvasWidth || animation.Config.Height!=canvasHeight {
  t.Fatalf("GIF shape mismatch: frames=%d size=%dx%d",len(animation.Image),animation.Config.Width,animation.Config.Height)
 }
 totalCentiseconds:=0
 for i,delay:=range animation.Delay {
  if delay<=0 {t.Errorf("frame %d has invalid delay %d",i,delay)}
  totalCentiseconds+=delay
 }
 if totalCentiseconds!=200 {t.Errorf("GIF duration got %d cs, want 200 cs",totalCentiseconds)}
}
