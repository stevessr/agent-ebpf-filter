package main

import (
	"strings"
	"testing"
	"time"
)

func TestWindowsObservationBaselineAndDeltas(t *testing.T) {
	now := time.Unix(1770000000, 0)
	first := windowsObservation{
		Processes: map[int]windowsProcessSample{7: {PID: 7, Name: "codex.exe", Start: 20}},
		Connections: map[string]windowsTCPSample{"7:a:b": {PID: 7, Local: "a", Remote: "1.2.3.4:443"}},
	}
	if got, dropped := windowsObservationEvents(windowsObservation{}, first, false, now); len(got) != 0 || dropped != 0 {
		t.Fatalf("baseline must not invent startup events: %v / %d", got, dropped)
	}
	next := windowsObservation{
		Processes: map[int]windowsProcessSample{7: {PID: 7, Name: "codex.exe", Start: 20}, 8: {PID: 8, PPID: 7, Name: "node.exe", Start: 30}},
		Connections: map[string]windowsTCPSample{"7:a:b": first.Connections["7:a:b"], "8:c:d": {PID: 8, Remote: "[::1]:443"}},
	}
	events, dropped := windowsObservationEvents(first, next, true, now)
	if len(events) != 2 || dropped != 0 {
		t.Fatalf("expected one process and one TCP observation: %#v, drops=%d", events, dropped)
	}
	if events[0].Type != "process_newly_observed" || events[0].PID != 8 || events[0].Decision != "OBSERVED" ||
		events[1].Type != "network_tcp_observed" || events[1].Target != "[::1]:443" {
		t.Fatalf("unexpected observed facts: %#v", events)
	}
	if !strings.HasPrefix(events[0].EventID, "windows:") || events[0].RiskScore != 0 {
		t.Fatal("sampling must not invent risk scores or kernel event IDs")
	}
	again, _ := windowsObservationEvents(next, next, true, now.Add(2*time.Second))
	if len(again) != 0 { t.Fatalf("stable snapshots must not duplicate observations: %#v", again) }
}

func TestWindowsObservationReuseAndDisappear(t *testing.T) {
	before := windowsObservation{Processes: map[int]windowsProcessSample{4: {PID: 4, Name: "a.exe", Start: 100}}, Connections: map[string]windowsTCPSample{}}
	after := windowsObservation{Processes: map[int]windowsProcessSample{4: {PID: 4, Name: "b.exe", Start: 200}}, Connections: map[string]windowsTCPSample{}}
	events, _ := windowsObservationEvents(before, after, true, time.Now())
	if len(events) != 2 || events[0].Type != "process_newly_observed" || events[1].Type != "process_disappeared" {
		t.Fatalf("PID reuse must not be treated as a stable process: %#v", events)
	}
}

func TestWindowsObservationBounded(t *testing.T) {
	next := windowsObservation{Processes: map[int]windowsProcessSample{}, Connections: map[string]windowsTCPSample{}}
	for i:=1;i<=windowsMaxEventsPerPoll+10;i++ { next.Processes[i] = windowsProcessSample{PID:i,Name:"test.exe"} }
	events, dropped := windowsObservationEvents(windowsObservation{Processes: map[int]windowsProcessSample{}}, next, true, time.Now())
	if len(events) != windowsMaxEventsPerPoll || dropped != 10 { t.Fatalf("bounded events: %d, dropped: %d", len(events), dropped) }
}

func TestWindowsCPUSampling(t *testing.T) {
	a:=windowsObservation{CPUTotal: 200, CPUIdle: 100}
	b:=windowsObservation{CPUTotal: 300, CPUIdle: 125}
	if got:=windowsCPUPercent(a,b); got != 75 {t.Fatalf("CPU: %.2f",got)}
	if got:=windowsProcessCPUPercent(windowsProcessSample{Start:1,CPU:0},windowsProcessSample{Start:2,CPU:999},time.Second,4); got != 0 {
		t.Fatalf("reused PID returned CPU: %.2f", got)
	}
}

func TestWindowsSampledObservationIsUnrated(t *testing.T) {
	event := eventSummary{Type: "network_tcp_observed", Decision: "OBSERVED"}
	if got := eventRisk(event); got != "未评级" {
		t.Fatalf("polling must not claim a connection is safe: %q", got)
	}
}


func TestWindowsObservationUDPBindingsAndPIDReuse(t *testing.T) {
    before := windowsObservation{
        Processes:map[int]windowsProcessSample{90:{PID:90,Name:"node.exe",Start:100}},
        Connections:map[string]windowsTCPSample{"tcp":{PID:90,Local:"0.0.0.0:1",Remote:"1.1.1.1:443"}},
        UDPBindings:map[string]windowsUDPSample{"udp":{PID:90,Local:"127.0.0.1:53"}},
    }
    after := windowsObservation{
        Processes:map[int]windowsProcessSample{90:{PID:90,Name:"node.exe",Start:200}},
        Connections:before.Connections, UDPBindings:before.UDPBindings,
    }
    events, dropped := windowsObservationEvents(before,after,true,time.Now())
    if len(events)!=4 || dropped!=0 { t.Fatalf("PID reuse: %#v, dropped %d",events,dropped) }
    if events[2].Type!="network_tcp_observed" || events[3].Type!="network_udp_binding_observed"{
        t.Fatalf("missed reused PID network identity: %#v",events)
    }
    stable,_:=windowsObservationEvents(after,after,true,time.Now())
    if len(stable)!=0 { t.Fatalf("unchanged sockets should not repeat: %#v",stable) }
}

func TestWindowsUDPInventorySearch(t *testing.T) {
    entries:=[]windowsUDPSample{{PID:20,Local:"[::1]:5353"},{PID:21,Local:"0.0.0.0:53"}}
    processes:=[]systemProcess{{PID:20,Name:"agent.exe"},{PID:21,Name:"dns.exe"}}
    if got:=windowsFilteredUDPBindings(entries,processes,"AGENT");len(got)!=1 || got[0].PID!=20{
        t.Fatalf("UDP process name filtering: %#v",got)
    }
    if got:=windowsFilteredUDPBindings(entries,processes,"0.0.0.0");len(got)!=1 || got[0].PID!=21{
        t.Fatalf("UDP endpoint filtering: %#v",got)
    }
    if got:=windowsFilteredUDPBindings(entries,processes,"");len(got)!=2{
        t.Fatalf("empty query should show all UDP bindings: %#v",got)
    }
}
