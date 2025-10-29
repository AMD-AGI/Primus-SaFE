package main

import (
	"context"
	"github.com/AMD-AGI/primus-lens/api/pkg/bootstrap"
)

func main() {
	err := bootstrap.StartServer(context.Background())
	if err != nil {
		panic(err)
	}
}
