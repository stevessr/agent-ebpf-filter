module github.com/stevessr/agent-ebpf-filter/desktop/renew

// MyGo is isolated here so desktop UI dependencies never enter the privileged backend module.
go 1.27.3

require (
	github.com/egoist/mygo v0.9.0
	github.com/gorilla/websocket v1.5.3
	github.com/pelletier/go-toml/v2 v2.4.3
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/cloudflare/circl v1.6.2 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	go.digitalxero.dev/rpm v0.3.4 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/image v0.47.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo
