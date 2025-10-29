package main

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/server"
	"github.com/AMD-AGI/primus-lens/primus-safe-adapter/pkg/bootstrap"
)

func main() {
	err := server.InitServerWithPreInitFunc(context.Background(), bootstrap.Init)
	if err != nil {
		panic(err)
	}
}
