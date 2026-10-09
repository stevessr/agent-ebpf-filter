package main

import "github.com/egoist/mygo/ui"

// Navigation icons use one consistent 24-DIP outline style. Parse them once:
// the native renderer can reuse each SVG mask across frames and theme changes.
func renewNavigationSVG(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var workspaceNavigationIcons = map[string]*ui.SVG{
	"概览": renewNavigationSVG(`<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="3" y="14" width="7" height="7" rx="1.5"/><rect x="14" y="14" width="7" height="7" rx="1.5"/>`),
	"研判": renewNavigationSVG(`<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5M3 16l9 5 9-5"/>`),
	"事件": renewNavigationSVG(`<path d="M8 6h13M8 12h13M8 18h13"/><path d="M3 6h.01M3 12h.01M3 18h.01"/>`),
	"会话": renewNavigationSVG(`<path d="M21 11.5a8.5 8.5 0 0 1-8.5 8.5 9 9 0 0 1-4.1-.9L3 21l1.9-5.4a9 9 0 0 1-.9-4.1A8.5 8.5 0 0 1 12.5 3 8.5 8.5 0 0 1 21 11.5Z"/><path d="M8 12h9"/>`),
	"域名": renewNavigationSVG(`<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c-3 4-3 14 0 18m0-18c3 4 3 14 0 18"/>`),
	"网络": renewNavigationSVG(`<circle cx="12" cy="5" r="2"/><circle cx="5" cy="18" r="2"/><circle cx="19" cy="18" r="2"/><path d="m11 7-5 9m7-9 5 9M7 18h10"/>`),
	"进程": renewNavigationSVG(`<rect x="8" y="2.5" width="8" height="6" rx="1.5"/><rect x="2" y="15.5" width="8" height="6" rx="1.5"/><rect x="14" y="15.5" width="8" height="6" rx="1.5"/><path d="M12 8.5V12M6 15.5V12h12v3.5"/>`),
	"Agent 识别": renewNavigationSVG(`<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="3"/><path d="M12 2v4m0 12v4M2 12h4m12 0h4"/>`),
	"监控": renewNavigationSVG(`<path d="M4 7h16M4 17h16"/><circle cx="9" cy="7" r="2.5" fill="white"/><circle cx="16" cy="17" r="2.5" fill="white"/>`),
	"eBPF 模块": renewNavigationSVG(`<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6" rx="1"/><path d="M9 2v3m6-3v3M9 19v3m6-3v3M2 9h3m-3 6h3m14-6h3m-3 6h3"/>`),
	"规则": renewNavigationSVG(`<path d="M12 2 4 5v6c0 5 3.4 8.5 8 11 4.6-2.5 8-6 8-11V5l-8-3Z"/><path d="m8 12 2.5 2.5L16 9"/>`),
	"跟踪": renewNavigationSVG(`<circle cx="6" cy="5" r="2"/><circle cx="18" cy="19" r="2"/><path d="M6 7v8a4 4 0 0 0 4 4h6M13 5h5a3 3 0 0 1 0 6h-5"/>`),
	"路径权限": renewNavigationSVG(`<path d="M3 8V6a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v4"/><path d="M3 8h18M3 8v10a2 2 0 0 0 2 2h6"/><rect x="13" y="15" width="8" height="6" rx="1"/><path d="M15 15v-2a2 2 0 0 1 4 0v2"/>`),
	"CCS": renewNavigationSVG(`<rect x="3" y="3" width="7" height="7" rx="1.5"/><rect x="14" y="3" width="7" height="7" rx="1.5"/><rect x="8.5" y="14" width="7" height="7" rx="1.5"/><path d="M6.5 10v2H12m5.5-2v2H12m0 0v2"/>`),
	"终端": renewNavigationSVG(`<rect x="2.5" y="4" width="19" height="16" rx="2"/><path d="m6 9 3 3-3 3m6 0h5"/>`),
	"系统": renewNavigationSVG(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.86l.05.05-2.9 2.9-.05-.05A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6v1h-4v-1a1.7 1.7 0 0 0-1-.6 1.7 1.7 0 0 0-1.86.34l-.05.05-2.9-2.9.05-.05A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1H3v-4h1a1.7 1.7 0 0 0 .6-1 1.7 1.7 0 0 0-.34-1.86l-.05-.05 2.9-2.9.05.05A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6V3h4v1a1.7 1.7 0 0 0 1 .6 1.7 1.7 0 0 0 1.86-.34l.05-.05 2.9 2.9-.05.05A1.7 1.7 0 0 0 19.4 9a1.7 1.7 0 0 0 .6 1h1v4h-1a1.7 1.7 0 0 0-.6 1Z"/>`),
}

func workspaceNavigationIcon(page string) *ui.SVG {
	return workspaceNavigationIcons[page]
}

// The narrow rail deliberately keeps the most-used pages. The full sidebar
// exposes the complete set, and both surfaces share these same SVGs.
var workspaceRailPages = []string{
	"概览", "事件", "研判", "会话", "网络", "域名", "进程",
	"Agent 识别", "监控", "规则", "系统", "终端",
}
