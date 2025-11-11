package main

import (
	"context"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/common/bootstrap"
)

func main() {
	err := bootstrap.Bootstrap(context.Background())
	if err != nil {
		log.Fatalf("Failed to bootstrap telemetry processor: %v", err)
	} else {
		log.Infof("Telemetry processor started successfully")
	}
}
