module github.com/stevessr/agent-ebpf-filter/mobile/monitor

go 1.27.3

// Upstream MyGo ships desktop runtimes only. The community fork provides
// Android's JNI host and native touch backend using the same mygo/ui API.
replace github.com/egoist/mygo => github.com/daodao97/mygo-mobile v0.0.0-20261112142321-f29b5777f85a

require github.com/egoist/mygo v0.7.4

require (
	github.com/Heyana/typesetting-go v0.3.5-heyana.4 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)
