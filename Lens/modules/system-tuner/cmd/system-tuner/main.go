package main

import (
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/system-tuner/pkg/tuner"
)

const (
	// CheckInterval check interval time
	CheckInterval = 30 * time.Second
)

func main() {
	fmt.Println("System-Tuner v0.2")

	// Create real file system and command executor
	fs := &tuner.OSFileSystem{}
	cmdExec := &tuner.OSCommandExecutor{}

	// Create system tuner with default config
	config := tuner.DefaultConfig()
	systemTuner := tuner.NewSystemTuner(config, fs, cmdExec)

	// Continuously check and tune system parameters
	for {
		// Check and set vm.max_map_count
		if err := systemTuner.CheckAndSetMaxMapCount(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		// Check and set max open files
		if err := systemTuner.CheckAndSetMaxOpenFiles(); err != nil {
			fmt.Printf("Error: %v\n", err)
		}

		// Wait for next check
		time.Sleep(CheckInterval)
	}
}
