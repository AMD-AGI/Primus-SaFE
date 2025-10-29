package main

import (
	"context"

	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/bootstrap"
)

func main() {
	err := bootstrap.Bootstrap(context.Background())
	if err != nil {
		panic(err)
	}
}
