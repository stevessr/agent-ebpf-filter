module agent-wrapper

go 1.26.2

require (
	agent-ebpf-filter v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.36.11
)

replace agent-ebpf-filter => ../backend
