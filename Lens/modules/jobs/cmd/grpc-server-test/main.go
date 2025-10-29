package main

import (
	"context"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/exporter"
	"time"
)

func main() {
	err := exporter.StartServer(context.Background(), 8991)
	if err != nil {
		panic(err)
	}
	time.Sleep(24 * time.Hour)
}
