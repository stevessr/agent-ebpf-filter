package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Event evidence and the system stream have different clocks and lifetimes.
// Never silently treat an arbitrary live PID as the historical event process.
type eventProcessReference struct {
	PID       int
	PPID      int
	Comm      string
	Occurred  time.Time
}

func processReferenceFromEvent(detail map[string]any) eventProcessReference {
	layers := eventDetailLayers(detail)
	get := func(keys ...string) string {
		value, _, _ := eventDetailLookupPreferred(layers, keys...)
		return value
	}
	parsePID := func(value string) int {
		n, err := strconv.Atoi(value)
		if err != nil || n <= 0 {
			return 0
		}
		return n
	}
	ref := eventProcessReference{
		PID:  parsePID(get("pid")),
		PPID: parsePID(get("ppid")),
		Comm: get("comm"),
	}
	// Persisted Timestamp is Unix milliseconds. Do not use monotonic kernel
	// nanoseconds as calendar time or as a PID-reuse identity check.
	if raw := mapValue(detail, "Timestamp"); raw != nil {
		if ms, ok := eventDetailInt64(detailScalarText(raw)); ok &&
			ms >= 946684800000 && ms <= 4102444800000 {
			ref.Occurred = time.UnixMilli(ms)
		}
	}
	if ref.Occurred.IsZero() {
		if nanos, ok := eventDetailInt64(get("timestampNs")); ok && isPlausibleUnixNanoseconds(nanos) {
			ref.Occurred = time.Unix(0, nanos)
		}
	}
	return ref
}

func processAtEvent(ref eventProcessReference, processes []systemProcess) (systemProcess, bool, string) {
	if ref.PID <= 0 {
		return systemProcess{}, false, "事件记录没有有效 PID，无法建立进程关联。"
	}
	for _, p := range processes {
		if p.PID != ref.PID {
			continue
		}
		if !ref.Occurred.IsZero() && p.CreateTime > 0 &&
			time.Unix(p.CreateTime, 0).After(ref.Occurred.Add(2*time.Second)) {
			return systemProcess{}, false, "当前 PID 已由较晚启动的进程占用（PID 复用），不能将它关联到历史事件。"
		}
		if ref.Occurred.IsZero() || p.CreateTime <= 0 {
			return p, true, "当前有同 PID 进程，但缺少可比对的事件时间或启动时间，身份尚未核实。"
		}
		return p, true, "当前 PID 的启动时间未晚于事件时间，但仅凭时间不能完全证明身份；下方是实时快照，不是历史进程树。"
	}
	return systemProcess{}, false, "实时进程列表中找不到该 PID；进程可能已退出，或快照尚未覆盖该进程。"
}

type eventProcessTreeRow struct {
	Process    systemProcess
	Depth      int
	ChildCount int
	OnPath     bool
	Expanded   bool
}

// Build a bounded, cycle-safe, focused live tree. Ancestors of the event PID
// stay visible; other branches can be expanded on demand. Never synthesize
// missing ancestors from a recycled PPID.
func buildEventProcessTreeRows(processes []systemProcess, targetPID int, expanded map[int]bool, limit int) ([]eventProcessTreeRow, bool) {
	if limit <= 0 {
		limit = 120
	}
	byPID := make(map[int]systemProcess, len(processes))
	children := make(map[int][]int, len(processes))
	for _, p := range processes {
		if p.PID > 0 {
			byPID[p.PID] = p
		}
	}
	if _, ok := byPID[targetPID]; !ok {
		return nil, false
	}
	// Index parents and sort for stable tree rows.
	for _, p := range byPID {
		if p.PPID > 0 && p.PPID != p.PID {
			if _, ok := byPID[p.PPID]; ok {
				children[p.PPID] = append(children[p.PPID], p.PID)
			}
		}
	}
	path := map[int]bool{}
	root := targetPID
	for steps := 0; steps < 40; steps++ {
		if path[root] {
			break
		}
		path[root] = true
		parent := byPID[root].PPID
		if parent <= 0 || parent == root {
			break
		}
		if _, ok := byPID[parent]; !ok || path[parent] {
			break
		}
		root = parent
	}
	// Prefer the route to the selected PID before rendering potentially
	// hundreds of siblings belonging to a high-level parent such as PID 1.
	for pid := range children {
		sort.Slice(children[pid], func(i, j int) bool {
			x, y := children[pid][i], children[pid][j]
			if path[x] != path[y] {
				return path[x]
			}
			return x < y
		})
	}
	rows := make([]eventProcessTreeRow, 0, min(limit, 64))
	visited := map[int]bool{}
	truncated := false
	var walk func(int, int)
	walk = func(pid, depth int) {
		if visited[pid] {
			return
		}
		if len(rows) >= limit {
			truncated = true
			return
		}
		visited[pid] = true
		descendants := children[pid]
		isOpen := path[pid]
		if value, set := expanded[pid]; set {
			isOpen = value
		}
		rows = append(rows, eventProcessTreeRow{
			Process: byPID[pid], Depth: depth, ChildCount: len(descendants),
			OnPath: path[pid], Expanded: isOpen,
		})
		if isOpen {
			for _, child := range descendants {
				walk(child, depth+1)
				if truncated {
					break
				}
			}
		}
	}
	walk(root, 0)
	return rows, truncated
}

func (a *renewApp) eventProcessInvestigation(c *ui.Context, detail map[string]any, height float32) {
	t := c.Theme()
	ref := processReferenceFromEvent(detail)
	var live []systemProcess
	if a.systemConnected {
		live = a.system.Processes
	}
	target, matched, matchNote := processAtEvent(ref, live)

	ui.Scroll(c).Height(height).Gap(12).Children(func() {
		card(c, "事件时的进程证据", func() {
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				if ref.PID > 0 {
					ui.Textf(c, "PID %d", ref.PID).Bold()
				} else {
					ui.Text(c, "未记录 PID").TextColor(t.Warning)
				}
				if ref.PPID > 0 {
					ui.Textf(c, "PPID %d", ref.PPID).Font("monospace")
				}
				if ref.Comm != "" {
					ui.Text(c, ref.Comm).Font("monospace")
				}
			})
			if !ref.Occurred.IsZero() {
				ui.Text(c, "事件时间："+ref.Occurred.Local().Format("2006-01-02 15:04:05.000")).FontSize(11).TextColor(t.TextMuted)
			}
			layers := eventDetailLayers(detail)
			for _, entry := range []struct{ label, key string }{
				{"UID", "uid"}, {"GID", "gid"}, {"TGID", "tgid"}, {"根 Agent PID", "rootAgentPid"},
				{"命令行", "commandLine"}, {"工作目录", "cwd"},
				{"容器 ID", "containerId"}, {"Cgroup ID", "cgroupId"},
			} {
				if value, origin, ok := eventDetailLookup(layers, entry.key); ok {
					ui.Row(c).Gap(8).Wrap().Children(func() {
						ui.Text(c, entry.label+": "+eventDetailPreview(value, 220)).Font("monospace").FontSize(11)
						ui.Text(c, origin).FontSize(10).TextColor(t.TextMuted)
					})
				}
			}
			if ref.PID > 0 && ui.Button(c, "筛选该 PID 的事件").Clicked() {
				a.openEventFilter(eventSummary{PID: ref.PID}, "pid")
				a.closeEventDetail()
			}
		})

		if !matched {
			ui.Text(c, matchNote).TextColor(t.Warning)
			if !a.systemConnected {
				ui.Text(c, "实时系统数据暂不可用；仅显示该事件实际保存的字段。").TextColor(t.TextMuted)
			}
			if ref.PPID > 0 && ref.PID > 0 {
				ui.Textf(c, "事件内记录的直接关系：PPID %d → PID %d（不能据此推断完整历史进程树）", ref.PPID, ref.PID).
					Font("monospace").FontSize(11)
			}
			return
		}
		ui.Text(c, matchNote).FontSize(11).TextColor(t.TextMuted)
		if ref.PPID > 0 && ref.PPID != target.PPID {
			ui.Textf(c, "注意：事件记录 PPID %d，当前快照 PPID %d；父进程可能发生变化。", ref.PPID, target.PPID).TextColor(t.Warning).FontSize(11)
		}
		if !a.system.FetchedAt.IsZero() {
			ui.Text(c, "快照时间："+a.system.FetchedAt.Local().Format("2006-01-02 15:04:05")).TextColor(t.TextMuted).FontSize(10)
		}

		ui.Tabs(c, &a.eventProcessTab, "进程树", "进程详细信息")
		if a.eventProcessTab == 0 {
			rows, truncated := buildEventProcessTreeRows(live, ref.PID, a.eventProcessExpanded, 120)
			if len(rows) == 0 {
				ui.Text(c, "当前进程关系不可用。").TextColor(t.TextMuted)
			}
			for _, row := range rows {
				p := row.Process
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, strings.Repeat("  ", min(row.Depth, 15))).Font("monospace")
					if row.ChildCount > 0 {
						label := "▾"
						if !row.Expanded {
							label = "▸"
						}
						if ui.Button(c, label).Tooltip("展开或收起子进程").Clicked() {
							if a.eventProcessExpanded == nil {
								a.eventProcessExpanded = make(map[int]bool)
							}
							a.eventProcessExpanded[p.PID] = !row.Expanded
						}
					} else {
						ui.Text(c, "·").Font("monospace")
					}
					text := fmt.Sprintf("%d  %s", p.PID, displayOr(p.Name, "未知进程"))
					if p.PID == ref.PID {
						ui.Text(c, text).Bold().TextColor(t.Accent).Grow(1).MinWidth(0)
					} else {
						ui.Text(c, text).Font("monospace").Grow(1).MinWidth(0)
					}
					if row.ChildCount > 0 {
						ui.Textf(c, "+%d", row.ChildCount).FontSize(10).TextColor(t.TextMuted)
					}
					if ui.Button(c, "详情").Clicked() {
						a.eventProcessSelectedPID = p.PID
						a.eventProcessTab = 1
					}
				})
			}
			if truncated {
				ui.Text(c, "进程树已限制为 120 行；收起较大分支以查看其他进程。").TextColor(t.Warning).FontSize(11)
			}
		} else {
			selectedPID := a.eventProcessSelectedPID
			if selectedPID == 0 {
				selectedPID = ref.PID
			}
			var current *systemProcess
			for i := range live {
				if live[i].PID == selectedPID {
					current = &live[i]
					break
				}
			}
			if current == nil {
				ui.Text(c, "选中进程已不在当前快照内，请返回进程树重新选择。").TextColor(t.Warning)
			} else {
				p := *current
				card(c, "进程详细信息 · 当前快照", func() {
					ui.Textf(c, "%s  ·  PID %d", displayOr(p.Name, "未知进程"), p.PID).FontSize(16).Bold()
					ui.Textf(c, "父 PID %d  ·  用户 %s", p.PPID, displayOr(p.User, "未提供")).FontSize(12)
					ui.Textf(c, "CPU %.1f%%  ·  内存 %.1f%%", p.CPU, p.MemPercent).FontSize(12)
					if p.CreateTime > 0 {
						ui.Text(c, "启动时间："+time.Unix(p.CreateTime, 0).Local().Format("2006-01-02 15:04:05")).FontSize(11)
					}
					count := 0
					for _, other := range live {
						if other.PPID == p.PID && other.PID != p.PID {
							count++
						}
					}
					ui.Textf(c, "当前直接子进程：%d", count).FontSize(11)
					ui.Text(c, "命令行").Bold().FontSize(12)
					ui.Text(c, displayOr(p.Cmdline, "当前快照未提供命令行")).Font("monospace").FontSize(11)
					ui.Row(c).Gap(8).Wrap().Children(func() {
						if ui.Button(c, "返回进程树").Clicked() {
							a.eventProcessTab = 0
						}
						if ui.Button(c, "复制 PID").Clicked() {
							c.WriteClipboard(strconv.Itoa(p.PID))
							c.Toast("已复制 PID")
						}
						if ui.Button(c, "查看该 PID 的事件").Clicked() {
							a.openEventFilter(eventSummary{PID: p.PID}, "pid")
							a.closeEventDetail()
						}
					})
				})
				if selectedPID != ref.PID {
					ui.Text(c, "当前选中的是关联节点，并非触发本事件的进程。").FontSize(11).TextColor(t.TextMuted)
				}
			}
		}
		ui.Text(c, "本视图不执行进程控制操作；当前快照与事件证据分别展示，历史关系缺失时不作推断。").
			FontSize(10).TextColor(t.TextMuted)
	})
}
