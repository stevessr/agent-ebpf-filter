module agent-ebpf-filter-tui

go 1.26.2

require (
	agent-ebpf-filter v0.0.0
	github.com/gdamore/tcell/v2 v2.13.9
	github.com/gorilla/websocket v1.5.3
	github.com/rivo/tview v0.42.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/term v0.37.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace agent-ebpf-filter => ../../backend
