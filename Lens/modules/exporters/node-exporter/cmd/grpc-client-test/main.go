package main

import (
	pb "github.com/AMD-AGI/primus-lens/core/pkg/pb/exporter"
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/collector/report"
	"time"
)

func main() {
	err := report.Init("127.0.0.1:8991", "test-node", "")
	if err != nil {
		panic(err)
	}
	err = report.GetStreamClient().Send(&pb.ContainerEvent{
		Type:        "test",
		ContainerId: "123",
		Data:        nil,
	})
	if err != nil {
		panic(err)
	}
	time.Sleep(1 * time.Second)
}
