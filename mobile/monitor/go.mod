module github.com/stevessr/agent-ebpf-filter/mobile/monitor

go 1.27.3

// Upstream MyGo ships desktop runtimes only. The community fork provides
// Android's JNI host and native touch backend using the same mygo/ui API.
replace github.com/egoist/mygo => github.com/daodao97/mygo-mobile v0.0.0-20261112142321-f29b5777f85a

require github.com/egoist/mygo v0.7.4
