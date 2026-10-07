module github.com/stevessr/agent-ebpf-filter/desktop/renew

// MyGo is isolated here so desktop UI dependencies never enter the privileged backend module.
go 1.27.1

require (
	github.com/egoist/mygo v0.2.18
	github.com/gorilla/websocket v1.5.3
	google.golang.org/protobuf v1.36.11
)

tool github.com/egoist/mygo/cmd/mygo
