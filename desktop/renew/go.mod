module github.com/stevessr/agent-ebpf-filter/desktop/renew

go 1.27.1

require (
	agent-ebpf-filter v0.0.0
	github.com/egoist/mygo v0.2.9
	github.com/gorilla/websocket v1.5.3
	google.golang.org/protobuf v1.36.11
)

replace agent-ebpf-filter => ../../backend

tool github.com/egoist/mygo/cmd/mygo
