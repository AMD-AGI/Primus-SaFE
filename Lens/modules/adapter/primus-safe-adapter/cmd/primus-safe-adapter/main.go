package main

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/primus-safe-adapter/pkg/bootstrap"
)

func main() {
	err := server.InitServerWithPreInitFunc(context.Background(), bootstrap.Init)
	if err != nil {
		panic(err)
	}
}
