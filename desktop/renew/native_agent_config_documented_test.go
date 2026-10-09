package main

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestDocumentedClaudeMCPAndSandbox(t *testing.T) {
 doc:=map[string]any{"permissions":map[string]any{"defaultMode":"bypassPermissions","disableBypassPermissionsMode":"disable","deny":[]any{"Read(/private)", "Bash(curl *)"}},"sandbox":map[string]any{"enabled":true,"failIfUnavailable":true,"allowUnsandboxedCommands":false,"excludedCommands":[]any{"docker *"}}}
 label:=claudeSafetySummary(doc)
 for _,part:=range []string{"权限绕过","禁止权限绕过","deny 规则 2","沙箱不可用","禁止命令逃逸","沙箱排除命令 1"} {
  if !strings.Contains(label,part){t.Fatalf("missing %q from %q",part,label)}
 }
 if strings.Contains(label,"/private")||strings.Contains(label,"curl"){t.Fatal("permission payload leaked")}
 path:=filepath.Join(t.TempDir(),".mcp.json")
 writeConfigFixture(t,path,`{"mcpServers":{"remote":{"url":"https://user:pass@invalid.example/x?token=SECRET"},"safe":{"type":"http","url":"https://mcp.example.org/events?key=SECRET","headers":{"Authorization":"SECRET"}},"stdio":{"command":"npx","args":["https://secret.example"],"env":{"KEY":"SECRET"}}}}`)
 rows,exists,err:=readClaudeMCP(path,"项目")
 if err!=nil||!exists||len(rows)!=3{t.Fatalf("MCP parse failed: %+v, %v",rows,err)}
 hosts:=map[string]string{}
 for _,r:=range rows{
  hosts[r.Provider]=r.Host
  if strings.Contains(r.Provider+r.Source+r.Security+r.Host,"SECRET")||strings.Contains(r.Provider+r.Source+r.Security+r.Host,"secret.example"){t.Fatal("MCP leaked secret")}
 }
 if hosts["MCP · safe"]!="mcp.example.org"||hosts["MCP · stdio"]!=""||hosts["MCP · remote"]!=""{t.Fatalf("MCP incorrect: %+v",hosts)}
}

func TestDocumentedCodexConfigMCPAndSecurity(t *testing.T) {
 doc:=map[string]any{
 "model_provider":"host-only","model_providers":map[string]any{"host-only":map[string]any{"base_url":"https://ignored.example.com"}},
 "sandbox_mode":"workspace-write","approval_policy":"on-request","default_permissions":":workspace",
 "permissions":map[string]any{"audit":map[string]any{"network":map[string]any{"enabled":true}}},
 "sandbox_workspace_write":map[string]any{"network_access":true},
 "mcp_servers":map[string]any{"serverA":map[string]any{"url":"https://codex-mcp.example.net/v2?key=SECRET"},"disabled":map[string]any{"url":"https://off.example.net/mcp","enabled":false},"local":map[string]any{"command":"bash","args":[]any{"curl https://secret.example"}}},
 }
 label:=codexSafetySummary(doc,true)
 for _,part:=range []string{"工作区写入","按需审批","权限配置 :workspace","沙箱网络访问","命名权限模板 1","无效的机器级覆盖键"}{
  if !strings.Contains(label,part){t.Fatalf("missing %q in %q",part,label)}
 }
 rows:=codexAdditionalRows(doc,"项目","config.toml",true)
 if len(rows)!=3||rows[0].Host!="off.example.net"||rows[1].Host!=""||rows[2].Host!="codex-mcp.example.net"||!strings.Contains(rows[0].Security,"已禁用"){t.Fatalf("MCP misparsed: %+v",rows)}
 for _,r:=range rows{if strings.Contains(r.Host+r.Security+r.Provider+r.Source,"SECRET"){t.Fatal("credential leaked")}}
}

func TestDocumentedCodexNestedProjectLayers(t *testing.T) {
 root:=filepath.Join(t.TempDir(),"repo")
 nested:=filepath.Join(root,"pkg","feature")
 if err:=os.MkdirAll(filepath.Join(root,".git"),0700);err!=nil{t.Fatal(err)}
 for _,p:=range []string{filepath.Join(root,".codex","config.toml"),filepath.Join(root,"pkg",".codex","config.toml"),filepath.Join(nested,".codex","config.toml")}{writeConfigFixture(t,p,"model = \"test\"\n")}
 got:=codexProjectLayers(nested)
 if len(got)!=3||got[0]!=filepath.Join(root,".codex","config.toml")||got[2]!=filepath.Join(nested,".codex","config.toml"){t.Fatalf("bad layer ordering %v",got)}
 if codexProjectLayers("relative")!=nil{t.Fatal("relative project accepted")}
}

func TestDocumentedConfigRejectsUnsafeURLsAndSymlinks(t *testing.T) {
 for _,raw:=range []string{"https://mcp.example.net:99999/v1","https://mcp.example.net:bad/v1","https://user:pw@mcp.example.net/v1","https://$TOKEN.example.com/mcp","javascript:alert(1)"}{
  if host,_:=safeSourceHost(raw);host!=""{t.Fatalf("accepted %q: %s",raw,host)}
 }
 dir:=t.TempDir()
 writeConfigFixture(t,filepath.Join(dir,"data.json"),"{}")
 if err:=os.Symlink(filepath.Join(dir,"data.json"),filepath.Join(dir,"link.json"));err!=nil{t.Fatal(err)}
 if _,_,err:=readClaudeMCP(filepath.Join(dir,"link.json"),"项目");err==nil{t.Fatal("followed symlink")}
}
