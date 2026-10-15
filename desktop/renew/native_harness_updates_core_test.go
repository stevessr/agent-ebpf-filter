package main

import (
 "errors"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
)

func fakeHarnessProbe(supported []string, outputs map[string]string, home string) harnessProbe {
 names:=map[string]bool{}
 for _,name:=range supported{names[name]=true}
 return harnessProbe{
  home:home,lookup:func(name string)(string,error){if !names[name]{return "",errors.New("missing")};return "/mock/bin/"+name,nil},
  resolve:filepath.EvalSymlinks,readFile:os.ReadFile,
  run:func(name string,args ...string)(string,error){
   key:=name+" "+strings.Join(args," ")
   if raw,ok:=outputs[key];ok{return raw,nil}
   return "",errors.New("not owned")
  },
 }
}

func TestHarnessUpdatePacmanOwnerBeforeOtherChannels(t *testing.T){
 dir:=t.TempDir()
 path:=filepath.Join(dir,"codex")
 if err:=os.WriteFile(path,[]byte("binary"),0700);err!=nil{t.Fatal(err)}
 p:=fakeHarnessProbe([]string{"pacman","sudo","npm"},map[string]string{"pacman -Qoq -- "+path:"codex-cli"},dir)
 got:=inspectHarnessInstallation("codex","Codex",path,p)
 if got.Source!="pacman"||got.Package!="codex-cli"{t.Fatalf("provenance: %+v",got)}
 want:=[]string{"/mock/bin/sudo","pacman","-S","--needed","--","codex-cli"}
 if !reflect.DeepEqual(got.Command,want){t.Fatalf("got %v want %v",got.Command,want)}
}

func TestHarnessUpdateNpmManifestAndGlobalRoot(t *testing.T){
 dir:=t.TempDir()
 root:=filepath.Join(dir,"npm","node_modules")
 pkgDir:=filepath.Join(root,"@openai","codex")
 if err:=os.MkdirAll(filepath.Join(pkgDir,"bin"),0755);err!=nil{t.Fatal(err)}
 real:=filepath.Join(pkgDir,"bin","codex.js")
 if err:=os.WriteFile(real,[]byte("#!/usr/bin/env node"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(pkgDir,"package.json"),[]byte("{\"name\":\"@openai/codex\"}"),0600);err!=nil{t.Fatal(err)}
 path:=filepath.Join(dir,"codex")
 if err:=os.Symlink(real,path);err!=nil{t.Fatal(err)}
 p:=fakeHarnessProbe([]string{"npm"},map[string]string{"npm root -g":root},dir)
 got:=inspectHarnessInstallation("codex","Codex",path,p)
 if got.Source!="npm"||got.Package!="@openai/codex"{t.Fatalf("provenance: %+v",got)}
 want:=[]string{"/mock/bin/npm","install","-g","@openai/codex@latest"}
 if !reflect.DeepEqual(got.Command,want){t.Fatalf("got %v want %v",got.Command,want)}
}

func TestHarnessUpdateProjectLocalNotGlobal(t *testing.T){
 dir:=t.TempDir()
 path:=filepath.Join(dir,"node_modules",".bin","codex")
 if err:=os.MkdirAll(filepath.Dir(path),0755);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("local"),0700);err!=nil{t.Fatal(err)}
 got:=inspectHarnessInstallation("codex","Codex",path,fakeHarnessProbe(nil,nil,dir))
 if got.Source!="project-local"||len(got.Command)!=0{t.Fatalf("unsafe project update: %+v",got)}
}

func TestHarnessUpdateOfficialClaudeRequiresVersionedDirectory(t *testing.T){
 dir:=t.TempDir()
 bin:=filepath.Join(dir,".local","share","claude","versions","2.4.0","claude")
 if err:=os.MkdirAll(filepath.Dir(bin),0755);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(bin,[]byte("native"),0700);err!=nil{t.Fatal(err)}
 p:=fakeHarnessProbe(nil,nil,dir)
 got:=inspectHarnessInstallation("claude","Claude Code",bin,p)
 if got.Source!="official-script"||!reflect.DeepEqual(got.Command,[]string{bin,"update"}){t.Fatalf("unexpected official: %+v",got)}
 other:=inspectHarnessInstallation("codex","Codex",bin,p)
 if other.Source!="unknown"||len(other.Command)>0{t.Fatalf("untrusted updater: %+v",other)}
}

func TestHarnessUpdateManagerCommands(t *testing.T){
 for _,tc:=range []struct{src,pkg,program,args string}{
  {"pnpm","@openai/codex","pnpm","add -g @openai/codex@latest"},
  {"npm","@openai/codex","npm","install -g @openai/codex@latest"},
  {"yarn","@openai/codex","yarn","global add @openai/codex@latest"},
  {"bun","@openai/codex","bun","add -g @openai/codex@latest"},
  {"flatpak","org.example.Codex","flatpak","update org.example.Codex"},
  {"apt","codex","sudo","apt-get install --only-upgrade -- codex"},
  {"pacman","codex","sudo","pacman -S --needed -- codex"},
  {"dnf","codex","sudo","dnf upgrade -- codex"},
  {"snap","codex","sudo","snap refresh codex"},
 }{
  t.Run(tc.src,func(t *testing.T){
   available:=[]string{tc.program}
   if tc.src=="dnf"{available=append(available,"dnf")}
   got:=harnessUpdateCommand(tc.src,tc.pkg,"",fakeHarnessProbe(available,nil,""))
   if len(got)==0||strings.Join(got[1:]," ")!=tc.args{t.Fatalf("update command: %v",got)}
  })
 }
}

func TestHarnessUpdateRejectsUnsafePackageNamesAndWrongDpkgPath(t *testing.T){
 p:=fakeHarnessProbe([]string{"npm"},nil,"")
 for _,value:=range []string{"../../evil","evil && whoami","--global","x\nhelp"}{
  if args:=harnessUpdateCommand("npm",value,"",p);len(args)!=0{t.Fatalf("accepted %q: %v",value,args)}
 }
 if got:=parseDpkgOwner("codex:amd64: /usr/bin/codex\n","/usr/bin/codex");got!="codex"{t.Fatal(got)}
 if got:=parseDpkgOwner("codex: /usr/bin/other","/usr/bin/codex");got!=""{t.Fatal(got)}
 if got:=parseDpkgOwner("evil;whoami: /usr/bin/codex","/usr/bin/codex");got!=""{t.Fatal(got)}
 missing:=inspectHarnessInstallation("gemini","Gemini CLI","",fakeHarnessProbe(nil,nil,""))
 if missing.Source!="missing"||len(missing.Command)>0{t.Fatal(missing)}
}
