package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type desktopProxyCaptureConfig struct {
	HTTPPort int `json:"httpPort"`
	SOCKS5Port int `json:"socks5Port"`
	SOCKSPort int `json:"socksPort"`
	InterceptTLS bool `json:"interceptTLS"`
	TLSAllowlist string `json:"tlsAllowlist"`
	JSONPreview bool `json:"jsonPreview"`
}
type desktopProxyCaptureStatus struct {
	Config desktopProxyCaptureConfig `json:"config"`
	Configured bool `json:"configured"`
	Running bool `json:"running"`
	CAReady bool `json:"caReady"`
	CAFingerprint string `json:"caFingerprint"`
	FlowCount int `json:"flowCount"`
}
type desktopProxyCaptureFlow struct {
	ID uint64 `json:"id"`
	At time.Time `json:"at"`
	Protocol string `json:"protocol"`
	Host string `json:"host"`
	Method string `json:"method"`
	Path string `json:"path"`
	Status int `json:"status"`
	DurationMS int64 `json:"durationMs"`
	RequestJSON string `json:"requestJson"`
	ResponseJSON string `json:"responseJson"`
}

func (c *apiClient) captureCert(ctx context.Context) ([]byte,error) {
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,c.origin+"/network/proxy-capture/ca",nil)
	if err!=nil{return nil,err}
	if c.token!=""{req.Header.Set("X-API-KEY",c.token);req.Header.Set("Authorization","Bearer "+c.token)}
	resp,err:=c.http.Do(req)
	if err!=nil{return nil,err}
	defer resp.Body.Close()
	if resp.StatusCode!=200{return nil,fmt.Errorf("CA download returned HTTP %d",resp.StatusCode)}
	return io.ReadAll(io.LimitReader(resp.Body,64<<10))
}

func (a *renewApp) proxyCaptureView(c *ui.Context) {
	t:=c.Theme()
	card(c,"本机代理与上下文捕获（mitmdump 风格）",func(){
		ui.Text(c,"按需配置 HTTP / SOCKS5 / SOCKS4 端口；仅监听 127.0.0.1，不会自动修改系统代理设置。").TextColor(t.TextMuted)
		ui.Text(c,"TLS 只解密白名单内的 HTTPS（包含对应 SOCKS CONNECT）；未解密隧道只记录目的地。").FontSize(11).TextColor(t.Warning)
		if !a.proxyCaptureLoaded && !a.proxyCaptureBusy {
			a.refreshProxyCapture()
		}
		if a.proxyCaptureError!="" {ui.Text(c,a.proxyCaptureError).TextColor(t.Danger)}
		if a.proxyCaptureInfo!="" {ui.Text(c,a.proxyCaptureInfo).TextColor(t.TextMuted)}
		if !a.proxyCaptureLoaded {ui.Text(c,"正在连接后端捕获服务…");return}
		s:=a.proxyCaptureStatus
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func(){
			if s.Running{statusPill(c,"捕获运行中",t.Success)}else{statusPill(c,"捕获已关闭",t.TextMuted)}
			ui.Text(c,fmt.Sprintf("内存记录 %d / 300",s.FlowCount)).TextColor(t.TextMuted)
			if ui.Button(c,"刷新").Clicked(){a.refreshProxyCapture()}
		})
		if !s.Running {
			ui.Row(c).Gap(10).Wrap().Children(func(){
				ui.TextInput(c,&a.proxyCaptureHTTP).Label("HTTP 端口 (0=关闭)").Width(140)
				ui.TextInput(c,&a.proxyCaptureSOCKS5).Label("SOCKS5 端口").Width(140)
				ui.TextInput(c,&a.proxyCaptureSOCKS).Label("SOCKS4/4a 端口").Width(140)
			})
			ui.Checkbox(c,&a.proxyCaptureTLS,"对指定 HTTPS 域名解密")
			if a.proxyCaptureTLS {
				ui.TextInput(c,&a.proxyCaptureAllowlist).Label("TLS 域名白名单 (逗号分隔，允许 *.example.com)").Grow(1)
			}
			ui.Checkbox(c,&a.proxyCapturePreview,"保存最多 4 KiB JSON 内容（即使遮蔽敏感字段仍有泄露风险）")
			if ui.Button(c,"保存代理端口与配置").Clicked() && !a.proxyCaptureBusy {
				a.configureProxyCapture()
			}
			if a.proxyCaptureStartConfirm {
				ui.Text(c,"确认已手动配置端口且仅捕获被授权的本机软件连接？").TextColor(t.Warning)
				if ui.Button(c,"确认启动捕获").Clicked() && !a.proxyCaptureBusy {
					a.proxyCaptureStartConfirm=false
					a.proxyCaptureAction(http.MethodPost,"/network/proxy-capture/start",map[string]any{"confirm":"I_UNDERSTAND_PROXY_CAPTURE"})
				}
				if ui.Button(c,"取消").Clicked(){a.proxyCaptureStartConfirm=false}
			} else if ui.Button(c,"启动前手动确认…").Clicked() && !a.proxyCaptureBusy {
				if !s.Configured {
					a.proxyCaptureError="请先填写并保存 HTTP / SOCKS 端口"
				} else if strings.TrimSpace(a.proxyCaptureHTTP)!=strconv.Itoa(s.Config.HTTPPort) ||
					strings.TrimSpace(a.proxyCaptureSOCKS5)!=strconv.Itoa(s.Config.SOCKS5Port) ||
					strings.TrimSpace(a.proxyCaptureSOCKS)!=strconv.Itoa(s.Config.SOCKSPort) ||
					a.proxyCaptureTLS!=s.Config.InterceptTLS ||
					a.proxyCaptureAllowlist!=s.Config.TLSAllowlist ||
					a.proxyCapturePreview!=s.Config.JSONPreview {
					a.proxyCaptureError="配置已修改，请先保存并等候后端确认，再启动捕获"
				} else {
					a.proxyCaptureStartConfirm=true
				}
			}
		} else if ui.Button(c,"停止捕获").Clicked() && !a.proxyCaptureBusy {
			a.proxyCaptureAction(http.MethodPost,"/network/proxy-capture/stop",map[string]any{})
		}
		ui.Text(c,"系统根证书 · 必须手动导入").Bold()
		if s.CAReady {
			ui.Text(c,"SHA-256: "+s.CAFingerprint).Font("monospace").FontSize(10).TextColor(t.TextMuted)
			ui.Text(c,"证书绝不会自动加入操作系统信任库；点击后会使用系统关联工具打开证书，仍需你手动导入并授权。").FontSize(11).TextColor(t.Warning)
			if a.proxyCaptureTrustConfirm {
				ui.Text(c,"只有在核对上方 SHA-256 指纹且理解 CA 信任风险后，才打开本机证书安装界面。")
				if ui.Button(c,"已确认：打开本机证书文件").Clicked() && !a.proxyCaptureBusy {
					a.proxyCaptureTrustConfirm=false
					a.openProxyCaptureCertificate()
				}
				if ui.Button(c,"取消证书导入").Clicked(){a.proxyCaptureTrustConfirm=false}
			} else if ui.Button(c,"手动确认并打开证书导入…").Clicked() {
				a.proxyCaptureTrustConfirm=true
			}
		} else {
			ui.Text(c,"尚无专用捕获 CA；生成根 CA 并不会安装或信任它。").TextColor(t.TextMuted)
			if a.proxyCaptureCAConfirm {
				ui.Text(c,"将创建 180 天有效的新 CA；私钥仅保存在后端本机。你确认吗？").TextColor(t.Warning)
				if ui.Button(c,"确认生成新的根 CA").Clicked() && !a.proxyCaptureBusy {
					a.proxyCaptureCAConfirm=false
					a.proxyCaptureAction(http.MethodPost,"/network/proxy-capture/ca/generate",map[string]any{"confirm":"GENERATE_CAPTURE_CA"})
				}
				if ui.Button(c,"取消").Clicked(){a.proxyCaptureCAConfirm=false}
			} else if ui.Button(c,"生成根证书（需确认）").Clicked() && !a.proxyCaptureBusy {
				a.proxyCaptureCAConfirm=true
			}
		}
		ui.Text(c,"近期会话 · 路径会隐藏查询参数").Bold()
		for i,flow:=range a.proxyCaptureFlows {
			if i>=12 {break}
			ui.Text(c,fmt.Sprintf("%s  %s  %s %s%s  %d  %d ms",
				flow.At.Local().Format("15:04:05"),flow.Protocol,flow.Method,flow.Host,flow.Path,flow.Status,flow.DurationMS)).
				Font("monospace").FontSize(11).TextColor(t.TextMuted)
			if flow.RequestJSON!="" {ui.Text(c,"  Request JSON: "+flow.RequestJSON).FontSize(10)}
			if flow.ResponseJSON!="" {ui.Text(c,"  Response JSON: "+flow.ResponseJSON).FontSize(10)}
		}
		if ui.Button(c,"清空内存会话").Clicked() && !a.proxyCaptureBusy {
			a.proxyCaptureAction(http.MethodDelete,"/network/proxy-capture/flows",nil)
		}
	})
}

func (a *renewApp) refreshProxyCapture() {
	if a.client==nil || a.proxyCaptureBusy {return}
	a.proxyCaptureBusy=true
	go func(){
		ctx,cancel:=context.WithTimeout(context.Background(),6*time.Second);defer cancel()
		var status desktopProxyCaptureStatus
		err:=a.client.getJSON(ctx,"/network/proxy-capture/status",&status)
		var data struct{ Flows []desktopProxyCaptureFlow `json:"flows"` }
		if err==nil {err=a.client.getJSON(ctx,"/network/proxy-capture/flows",&data)}
		a.update(func(){
			a.proxyCaptureBusy=false
			if err!=nil {a.proxyCaptureError=err.Error();return}
			if !a.proxyCaptureLoaded {
				a.proxyCaptureHTTP=strconv.Itoa(status.Config.HTTPPort)
				a.proxyCaptureSOCKS5=strconv.Itoa(status.Config.SOCKS5Port)
				a.proxyCaptureSOCKS=strconv.Itoa(status.Config.SOCKSPort)
				a.proxyCaptureTLS=status.Config.InterceptTLS
				a.proxyCaptureAllowlist=status.Config.TLSAllowlist
				a.proxyCapturePreview=status.Config.JSONPreview
			}
			a.proxyCaptureStatus=status
			a.proxyCaptureFlows=data.Flows
			a.proxyCaptureLoaded=true
			a.proxyCaptureError=""
		})
	}()
}

func (a *renewApp) configureProxyCapture() {
	httpPort,e1:=strconv.Atoi(strings.TrimSpace(a.proxyCaptureHTTP))
	socks5Port,e2:=strconv.Atoi(strings.TrimSpace(a.proxyCaptureSOCKS5))
	socksPort,e3:=strconv.Atoi(strings.TrimSpace(a.proxyCaptureSOCKS))
	if e1!=nil||e2!=nil||e3!=nil {a.proxyCaptureError="端口必须是数字；不启用的协议请填写 0";return}
	a.proxyCaptureAction(http.MethodPut,"/network/proxy-capture/config",desktopProxyCaptureConfig{
		HTTPPort:httpPort,SOCKS5Port:socks5Port,SOCKSPort:socksPort,
		InterceptTLS:a.proxyCaptureTLS,TLSAllowlist:a.proxyCaptureAllowlist,JSONPreview:a.proxyCapturePreview,
	})
}

func (a *renewApp) proxyCaptureAction(method,path string,body any) {
	if a.client==nil||a.proxyCaptureBusy {return}
	a.proxyCaptureBusy=true
	a.proxyCaptureError=""
	go func(){
		ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
		err:=a.client.requestJSON(ctx,method,path,body,nil)
		a.update(func(){
			a.proxyCaptureBusy=false
			if err!=nil {a.proxyCaptureError=err.Error();return}
			a.proxyCaptureInfo="操作已由后端确认"
			a.proxyCaptureLoaded=false
			a.refreshProxyCapture()
		})
	}()
}

func (a *renewApp) openProxyCaptureCertificate() {
	if a.client==nil || a.proxyCaptureBusy {return}
	a.proxyCaptureBusy=true
	go func(){
		ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
		data,err:=a.client.captureCert(ctx)
		var path string
		if err==nil {
			file,e:=os.CreateTemp("","agent-ebpf-capture-ca-*.cer")
			if e!=nil{err=e}else{
				path=file.Name()
				_,err=file.Write(data)
				closeErr:=file.Close()
				if err==nil {err=closeErr}
				if err==nil {err=openNativeCertificateImport(path)}
				if err!=nil {_=os.Remove(path)}
			}
		}
		a.update(func(){
			a.proxyCaptureBusy=false
			if err!=nil {a.proxyCaptureError=err.Error();return}
			a.proxyCaptureInfo="已打开证书查看/导入界面；仅当你在系统中批准安装后证书才会受到信任。临时证书："+filepath.Base(path)
		})
	}()
}

func openNativeCertificateImport(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd=exec.Command("rundll32.exe","cryptext.dll,CryptExtAddCER",path)
	case "darwin":
		cmd=exec.Command("open",path)
	default:
		cmd=exec.Command("xdg-open",path)
	}
	if err:=cmd.Start();err!=nil{return fmt.Errorf("open OS certificate manager: %w; import certificate manually from %s",err,path)}
	return cmd.Process.Release()
}
