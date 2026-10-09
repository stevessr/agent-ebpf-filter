module github.com/stevessr/agent-ebpf-filter/desktop/renew

// MyGo is isolated here so desktop UI dependencies never enter the privileged backend module.
go 1.27.1

require (
	github.com/egoist/mygo v0.3.5
	github.com/gorilla/websocket v1.5.3
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo
