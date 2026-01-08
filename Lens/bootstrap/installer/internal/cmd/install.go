// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/workflow"
)

var (
	dryRun  bool
	timeout time.Duration
)

// installCmd represents the install command
var installCmd = &cobra.Command{
	Use:   "install [workflow]",
	Short: "Install Primus Lens components",
	Long: `Install Primus Lens components using the specified workflow.

Available workflows:
  dataplane    - Install data plane components (operators, infrastructure, applications)
  controlplane - Install control plane components (API, Adapter)
  standalone   - Install all components in standalone mode

Examples:
  # Install dataplane with default config
  primus-lens-installer install dataplane

  # Install with custom config
  primus-lens-installer install dataplane --config my-values.yaml

  # Dry-run mode (show what would be done)
  primus-lens-installer install dataplane --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func init() {
	rootCmd.AddCommand(installCmd)

	installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "simulate installation without making changes")
	installCmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "overall installation timeout")
}

// ValuesFileSetter is implemented by workflows that support setting a values file
type ValuesFileSetter interface {
	SetValuesFile(file string)
}

// ChartsDirSetter is implemented by workflows that support setting charts directory
type ChartsDirSetter interface {
	SetChartsDir(dir string)
}

func runInstall(cmd *cobra.Command, args []string) error {
	workflowName := args[0]

	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create workflow
	wf, err := workflow.NewWorkflow(workflowName, cfg)
	if err != nil {
		return fmt.Errorf("failed to create workflow: %w", err)
	}

	// Set values file if workflow supports it and config file is specified
	if cfgFile != "" {
		if setter, ok := wf.(ValuesFileSetter); ok {
			setter.SetValuesFile(cfgFile)
		}
	}

	// Set charts directory from environment variable if available
	if chartsDir := os.Getenv("CHARTS_DIR"); chartsDir != "" {
		if setter, ok := wf.(ChartsDirSetter); ok {
			setter.SetChartsDir(chartsDir)
		}
	}

	// Setup context with timeout and signal handling
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, cancelling...")
		cancel()
	}()

	// Run installation
	opts := workflow.RunOptions{
		DryRun:     dryRun,
		Verbose:    verbose,
		Kubeconfig: getKubeconfig(),
		Namespace:  namespace,
	}

	fmt.Printf("Starting %s installation...\n", workflowName)
	fmt.Printf("Namespace: %s\n", namespace)
	if dryRun {
		fmt.Println("Mode: DRY-RUN (no changes will be made)")
	}
	fmt.Println()

	if err := wf.Install(ctx, opts); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	fmt.Println()
	fmt.Printf("✅ %s installation completed successfully!\n", workflowName)
	return nil
}

func loadConfig() (*config.Config, error) {
	configFile := cfgFile
	if configFile == "" {
		// Try default locations
		candidates := []string{
			"values.yaml",
			"../charts/primus-lens-dataplane/values.yaml",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				configFile = c
				break
			}
		}
	}

	if configFile == "" {
		// Return default config if no file found
		return config.DefaultConfig(), nil
	}

	return config.LoadFromFile(configFile)
}
