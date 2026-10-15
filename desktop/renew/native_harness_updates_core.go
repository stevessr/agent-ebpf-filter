package main

// Local installation provenance detection. Never execute remote installer scripts.
import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "runtime"
 "strings"
 "time"
)

type harnessUpdatePlan struct {
 ID, Label, Path, Source, Package, Evidence, Advisory string
 Command []string
}

var harnessUpdatePackageNames=map[string]string{
 "codex":"@openai/codex",
 "claude":"@anthropic-ai/claude-code",
 "gemini":"@google/gemini-cli",
 "opencode":"opencode-ai",
 "pi":"@mariozechner/pi-coding-agent",
 "copilot":"@github/copilot",
 "deepseek":"@deepseek-ai/dsh",
}

var safeOwnedPackage=regexp.MustCompile("^(?:@[A-Za-z0-9._-]+/)?[A-Za-z0-9][A-Za-z0-9+_.@-]{0,127}$")

type harnessProbe struct {
 lookup func(string)(string,error)
 resolve func(string)(string,error)
 readFile func(string)([]byte,error)
 run func(string,...string)(string,error)
 home string
 goos string
}

func systemHarnessProbe() harnessProbe {
 home,_:=os.UserHomeDir()
 return harnessProbe{
  lookup:exec.LookPath,resolve:filepath.EvalSymlinks,readFile:os.ReadFile,home:home,goos:runtime.GOOS,
  run:func(name string,args ...string)(string,error){
   ctx,cancel:=context.WithTimeout(context.Background(),1600*time.Millisecond)
   defer cancel()
   output,err:=exec.CommandContext(ctx,name,args...).CombinedOutput()
   if err!=nil{return "",err}
   if len(output)>8192{return "",fmt.Errorf("package metadata too large")}
   return strings.TrimSpace(string(output)),nil
  },
 }
}
func (p harnessProbe) has(name string)bool{_,err:=p.lookup(name);return err==nil}
func (p harnessProbe) use(name string)string{path,err:=p.lookup(name);if err!=nil{return ""};return path}
func underDirectory(path,dir string)bool{
 if dir==""||path==""{return false}
 rel,err:=filepath.Rel(filepath.Clean(dir),filepath.Clean(path))
 return err==nil&&rel!="."&&rel!=".."&&!strings.HasPrefix(rel,".."+string(filepath.Separator))
}
func cleanOwnedName(name string)string{
 name=strings.TrimSpace(name)
 if !safeOwnedPackage.MatchString(name){return ""}
 return name
}
func parseDpkgOwner(raw,path string)string{
 for _,line:=range strings.Split(raw,"\n"){
  parts:=strings.SplitN(line,": ",2)
  if len(parts)!=2||filepath.Clean(strings.TrimSpace(parts[1]))!=filepath.Clean(path){continue}
  name:=strings.TrimSpace(strings.Split(parts[0],",")[0])
  if colon:=strings.IndexByte(name,':');colon>=0{name=name[:colon]}
  if name=cleanOwnedName(name);name!=""{return name}
 }
 return ""
}
func (p harnessProbe) systemOwner(path string)(source,pkg string){
 if p.has("pacman"){
  if raw,err:=p.run("pacman","-Qoq","--",path);err==nil{
   if name:=cleanOwnedName(strings.TrimSpace(raw));name!=""{return "pacman",name}
  }
 }
 if p.has("dpkg-query")&&p.has("apt-get"){
  if raw,err:=p.run("dpkg-query","-S",path);err==nil{
   if name:=parseDpkgOwner(raw,path);name!=""{return "apt",name}
  }
 }
 if p.has("rpm")&&(p.has("dnf")||p.has("dnf5")){
  if raw,err:=p.run("rpm","-qf","--qf","%{NAME}\n",path);err==nil{
   if name:=cleanOwnedName(strings.TrimSpace(raw));name!=""{return "dnf",name}
  }
 }
 return "",""
}

// Snap/Flatpak require both an exported executable and package metadata.
func (p harnessProbe) sandboxOwner(path string)(source,pkg string){
 if p.has("snap")&&(underDirectory(path,"/snap/bin")||underDirectory(path,"/var/lib/snapd/snap/bin")){
  candidate:=strings.SplitN(filepath.Base(path),".",2)[0]
  if cleanOwnedName(candidate)!=""{
   if raw,err:=p.run("snap","list",candidate);err==nil{
    lines:=strings.Split(raw,"\n")
    if len(lines)>1&&len(strings.Fields(lines[1]))>0&&strings.Fields(lines[1])[0]==candidate{return "snap",candidate}
   }
  }
 }
 bases:=[]string{"/var/lib/flatpak/exports/bin"}
 if p.home!=""{bases=append(bases,filepath.Join(p.home,".local/share/flatpak/exports/bin"))}
 for _,base:=range bases{
  if !underDirectory(path,base){continue}
  app:=filepath.Base(path)
  if cleanOwnedName(app)==""||!strings.Contains(app,"."){continue}
  if p.has("flatpak"){
   if _,err:=p.run("flatpak","info",app);err==nil{return "flatpak",app}
  }
 }
 return "",""
}
func (p harnessProbe) nodePackage(path,expected string)string{
 if expected==""{return ""}
 for current,i:=filepath.Dir(path),0;i<14;i++{
  bytes,err:=p.readFile(filepath.Join(current,"package.json"))
  if err==nil{
   var manifest map[string]any
   if json.Unmarshal(bytes,&manifest)==nil&&manifest["name"]==expected{return current}
  }
  next:=filepath.Dir(current)
  if next==current{break}
  current=next
 }
 return ""
}
func (p harnessProbe) globalNodeSource(path,pkgDir string)string{
 for _,item:=range []struct{program string;args []string;source string;appendNodeModules bool}{
  {"pnpm",[]string{"root","-g"},"pnpm",false},
  {"yarn",[]string{"global","dir"},"yarn",true},
  {"npm",[]string{"root","-g"},"npm",false},
 }{
  if !p.has(item.program){continue}
  root,err:=p.run(item.program,item.args...)
  if err!=nil{continue}
  root=strings.TrimSpace(root)
  if item.appendNodeModules{root=filepath.Join(root,"node_modules")}
  if !filepath.IsAbs(root){continue}
  if underDirectory(path,root)&&underDirectory(pkgDir,root){return item.source}
 }
 bunBase:=""
 if p.home!=""{bunBase=filepath.Join(p.home,".bun")}
 if configured:=os.Getenv("BUN_INSTALL");configured!=""&&filepath.IsAbs(configured){bunBase=configured}
 if p.has("bun")&&bunBase!=""{
  root:=filepath.Join(bunBase,"install","global","node_modules")
  if underDirectory(path,root)&&underDirectory(pkgDir,root){return "bun"}
 }
 return ""
}
// pnpm's global launcher can be a generated shell shim rather than a symlink.
// Confirm all three signals: executable is in the global bin directory, pnpm
// reports the exact package as globally installed, and the shim names it.
func (p harnessProbe) pnpmGlobalShim(path,expected string)bool{
 if expected==""||!p.has("pnpm"){return false}
 binDir,err:=p.run("pnpm","bin","-g")
 if err!=nil||!filepath.IsAbs(strings.TrimSpace(binDir)){return false}
 if filepath.Clean(path)!=filepath.Join(strings.TrimSpace(binDir),filepath.Base(path)){return false}
 inventory,err:=p.run("pnpm","list","-g","--depth","0","--json")
 if err!=nil{return false}
 var projects []struct{Dependencies map[string]json.RawMessage}
 if json.Unmarshal([]byte(inventory),&projects)!=nil{return false}
 found:=false
 for _,project:=range projects{if _,ok:=project.Dependencies[expected];ok{found=true;break}}
 if !found{return false}
 script,err:=p.readFile(path)
 if err!=nil||len(script)>32768{return false}
 readable:=string(script)
 return strings.Contains(readable,expected)||strings.Contains(readable,strings.ReplaceAll(expected,"/","+"))
}

func harnessUpdateCommand(source,pkg,executable string,p harnessProbe)[]string{
 if cleanOwnedName(pkg)==""{return nil}
 switch source{
 case "pnpm":if bin:=p.use("pnpm");bin!=""{return []string{bin,"add","-g",pkg+"@latest"}}
 case "npm":if bin:=p.use("npm");bin!=""{return []string{bin,"install","-g",pkg+"@latest"}}
 case "yarn":if bin:=p.use("yarn");bin!=""{return []string{bin,"global","add",pkg+"@latest"}}
 case "bun":if bin:=p.use("bun");bin!=""{return []string{bin,"add","-g",pkg+"@latest"}}
 case "flatpak":if bin:=p.use("flatpak");bin!=""{return []string{bin,"update",pkg}}
 case "apt":if p.has("sudo"){return []string{p.use("sudo"),"apt-get","install","--only-upgrade","--",pkg}}
 case "pacman":if p.has("sudo"){return []string{p.use("sudo"),"pacman","-S","--needed","--",pkg}}
 case "dnf":if p.has("sudo"){manager:="dnf";if !p.has(manager){manager="dnf5"};return []string{p.use("sudo"),manager,"upgrade","--",pkg}}
 case "snap":if p.has("sudo"){return []string{p.use("sudo"),"snap","refresh",pkg}}
 case "official-script":if executable!=""&&pkg=="claude"{return []string{executable,"update"}}
 case "winget":if bin:=p.use("winget");bin!=""{return []string{bin,"upgrade","--id",pkg,"--exact","--source","winget"}}
 case "msstore":if bin:=p.use("winget");bin!=""{return []string{bin,"upgrade","--id",pkg,"--exact","--source","msstore"}}
 case "scoop":if p.has("scoop")&&p.has("powershell.exe"){return []string{p.use("powershell.exe"),"-NoProfile","-NonInteractive","-Command","scoop update "+pkg}}
 case "choco":if bin:=p.use("choco");bin!=""{return []string{bin,"upgrade",pkg}}
 }
 return nil
}
func inspectHarnessInstallation(id,label,path string,p harnessProbe)harnessUpdatePlan{
 result:=harnessUpdatePlan{ID:id,Label:label,Path:path,Source:"unknown",Advisory:"安装来源无法可靠确认；请使用原安装渠道手动更新，避免覆盖或重复安装。"}
 if path==""{result.Source="missing";result.Advisory="尚未在 PATH 中发现该 CLI。";return result}
 absolute,err:=filepath.Abs(path)
 if p.goos=="windows"&&len(path)>=3&&path[1]==':'&&(path[2]=='\\'||path[2]=='/'){absolute,err=path,nil}
 if err!=nil{return result}
 resolved,err:=p.resolve(absolute)
 if err!=nil{resolved=absolute}
 for _,owned:=range []string{absolute,resolved}{
  if p.goos=="windows"{
   if src,pkg,evidence:=p.windowsCLIOwner(id,owned);src!=""{
    result.Source,result.Package,result.Evidence=src,pkg,evidence
    result.Command=harnessUpdateCommand(src,pkg,"",p)
    result.Advisory="Windows 包管理器已核对已安装包与入口；请确认预览的单包更新命令。"
    return result
   }
  }
  if src,pkg:=p.sandboxOwner(owned);src!=""{
   result.Source,result.Package,result.Evidence=src,pkg,"包管理器确认了 PATH 导出的应用入口"
   result.Command=harnessUpdateCommand(src,pkg,"",p)
   result.Advisory="只更新当前应用；发行渠道决定可用版本。"
   return result
  }
  if src,pkg:=p.systemOwner(owned);src!=""{
   result.Source,result.Package,result.Evidence=src,pkg,"系统包数据库确认文件归属"
   result.Command=harnessUpdateCommand(src,pkg,"",p)
   result.Advisory="仅升级对应系统包，不切换安装来源；可能需要 sudo 授权。"
   return result
  }
 }
 if strings.Contains(absolute,string(filepath.Separator)+"node_modules"+string(filepath.Separator)+".bin"+string(filepath.Separator)){
  result.Source="project-local";result.Advisory="这是项目内局部安装，请在项目内更新依赖与锁文件。";return result
 }
 expected:=harnessUpdatePackageNames[id]
 if pkgDir:=p.nodePackage(resolved,expected);pkgDir!=""{
  if source:=p.globalNodeSource(resolved,pkgDir);source!=""{
   result.Source,result.Package,result.Evidence=source,expected,"全局包目录与 package.json 名称均匹配"
   result.Command=harnessUpdateCommand(source,expected,"",p)
   result.Advisory="按当前全局安装器更新，不切换来源；执行前请确认命令。"
   return result
  }
  result.Advisory="已识别 Node 包，但尚不能确认原始包管理器；需要人工核对。"
  return result
 }
 // A pnpm global shim is not necessarily a symlink to its real package.
 for _,launcher:=range []string{absolute,resolved}{
  if p.pnpmGlobalShim(launcher,expected){
   result.Source,result.Package,result.Evidence="pnpm",expected,"pnpm 全局清单、bin 目录及启动脚本一致"
   result.Command=harnessUpdateCommand("pnpm",expected,"",p)
   result.Advisory="使用原 pnpm 全局安装渠道更新；执行前请确认命令。"
   return result
  }
 }
 // Official Claude native installer owns this version-managed directory.
 // Generic ~/.local/bin files are deliberately NOT treated as official.
 if id=="claude"&&p.home!=""&&(underDirectory(resolved,filepath.Join(p.home,".local","share","claude","versions"))||underDirectory(resolved,filepath.Join(p.home,".claude","local"))){
  result.Source,result.Package,result.Evidence="official-script","claude","可执行文件位于 Claude 官方版本管理目录"
  result.Command=harnessUpdateCommand(result.Source,"claude",absolute,p)
  result.Advisory="调用官方 CLI 内建更新命令；不执行 curl | sh 或未审核脚本。"
 }
 return result
}
func scanHarnessUpdatePlans()[]harnessUpdatePlan{
 p:=systemHarnessProbe()
 results:=make([]harnessUpdatePlan,0,len(codingHarnessCatalog)+1)
 for _,spec:=range codingHarnessCatalog{
  path,_:=p.lookup(spec.Command)
  results=append(results,inspectHarnessInstallation(spec.ID,spec.Label,path,p))
 }
 path,_:=p.lookup("dsh")
 results=append(results,inspectHarnessInstallation("deepseek","DeepSeek Harness",path,p))
 return append(results,scanDesktopAppUpdates(p)...)
}
func updatePreview(command []string)string{return strings.Join(command," ")}
