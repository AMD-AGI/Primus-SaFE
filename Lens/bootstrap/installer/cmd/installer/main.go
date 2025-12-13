package main

import (
	"os"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
