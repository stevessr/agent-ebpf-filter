package executiongraph

import (
	"time"

	"agent-ebpf-filter/pb"
)

type Record struct {
	Event      *pb.Event
	ReceivedAt time.Time
}
