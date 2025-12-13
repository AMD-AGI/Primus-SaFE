package main

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/bootstrap"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

func main() {
	err := bootstrap.Bootstrap(context.Background())
	if err != nil {
		log.Fatalf("Failed to bootstrap AI Advisor: %v", err)
	} else {
		log.Infof("AI Advisor started successfully")
	}
}
