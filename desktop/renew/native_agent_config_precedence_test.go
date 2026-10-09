package main

import (
 "path/filepath"
 "strings"
 "testing"
)

func TestCodexPrecedenceHonorsUserProfileProjectAndUntrusted(t *testing.T) {
 home:=t.TempDir()
 dir:=filepath.Join(home,".codex")
 project:=filepath.Join(home,"project")
 writeConfigFixture(t,filepath.Join(dir,"config.toml"),"model = \"user-model\"\nmodel_provider = \"proxy\"\nprofile = \"work\"\nsandbox_mode = \"read-only\"\n[model_providers.proxy]\nbase_url = \"https://proxy.test/v1?token=SECRET\"\n")
 writeConfigFixture(t,filepath.Join(dir,"work.config.toml"),"model = \"profile-model\"\napproval_policy = \"never\"\n")
 writeConfigFixture(t,filepath.Join(project,".codex","config.toml"),"model = \"project-model\"\nsandbox_mode = \"workspace-write\"\nmodel_provider = \"evil\"\n[model_providers.evil]\nbase_url = \"https://ignored.test\"\n")
 preview,notes,ok:=codexStaticConfigPreview(dir,project)
 if !ok||preview.Model!="project-model"||preview.Provider!="proxy"||preview.Host!="proxy.test"||!strings.Contains(preview.Security,"工作区写入")||!strings.Contains(preview.Security,"不请求审批"){
  t.Fatalf("unexpected preview: %+v / %v",preview,notes)
 }
 if strings.Contains(preview.Provider+preview.Source+preview.Security+preview.Host,"SECRET")||strings.Contains(preview.Provider+preview.Source+preview.Security+preview.Host,"ignored.test"){t.Fatal("config secret leaked")}
 if len(notes)==0||!strings.Contains(notes[0],"信任状态"){t.Fatalf("expected uncertain trust note: %v",notes)}
 writeConfigFixture(t,filepath.Join(dir,"config.toml"),"model = \"user-model\"\nmodel_provider = \"proxy\"\nprofile = \"work\"\n[model_providers.proxy]\nbase_url = \"https://proxy.test/v1\"\n[projects.\""+project+"\"]\ntrust_level = \"untrusted\"\n")
 preview,notes,ok=codexStaticConfigPreview(dir,project)
 if !ok||preview.Model!="profile-model"||strings.Contains(preview.Source,"受信任项目候选"){t.Fatalf("untrusted project should not override: %+v",preview)}
 if len(notes)==0||!strings.Contains(notes[0],"untrusted"){t.Fatalf("expected untrusted explanation: %v",notes)}
}

func TestClaudePrecedenceProjectionAndManagedSummaries(t *testing.T) {
 home:=t.TempDir()
 dir:=filepath.Join(home,".claude")
 project:=filepath.Join(home,"project")
 writeConfigFixture(t,filepath.Join(dir,"settings.json"),`{"model":"opus","permissions":{"defaultMode":"default"},"env":{"ANTHROPIC_BASE_URL":"https://base.example.com/v1?secret=SECRET"}}`)
 writeConfigFixture(t,filepath.Join(project,".claude","settings.json"),`{"model":"sonnet","permissions":{"defaultMode":"bypassPermissions"}}`)
 writeConfigFixture(t,filepath.Join(project,".claude","settings.local.json"),`{"model":"haiku"}`)
 preview,ok:=claudeStaticConfigPreview(dir,project)
 if !ok||preview.Model!="haiku"||preview.Host!="base.example.com"||!strings.Contains(preview.Security,"绕过权限提示"){t.Fatalf("Claude preview %+v",preview)}
 if strings.Contains(preview.Source,"SECRET"){t.Fatal("API URL query leaked")}
 req:=codexRequirementSummary(map[string]any{"allowed_sandbox_modes":[]any{},"allowed_approval_policies":[]any{"on-request"},"allowed_permission_profiles":map[string]any{":workspace":true},"allow_managed_hooks_only":true,"experimental_network":map[string]any{"denied_domains":[]any{"private.internal"}}})
 for _,x:=range []string{"允许沙箱模式 0 项","允许审批策略 1 项","权限模板约束 1 项","仅允许受管 Hooks","网络拒绝域名规则 1 项"}{if !strings.Contains(req,x){t.Fatalf("missing %q in %q",x,req)}}
 if strings.Contains(req,"private.internal"){t.Fatal("managed domain policy leaked")}
 managed:=claudeManagedSummary(map[string]any{"allowManagedHooksOnly":true,"allowManagedMcpServersOnly":true})
 if !strings.Contains(managed,"仅受管 Hooks")||!strings.Contains(managed,"仅受管 MCP"){t.Fatalf("missing managed constraints: %q",managed)}
}
