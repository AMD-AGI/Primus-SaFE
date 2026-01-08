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

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/workflow"
)

var (
	force         bool
	deleteData    bool
	uninstallTimeout time.Duration
)

// uninstallCmd represents the uninstall command
var uninstallCmd = &cobra.Command{
	Use:   "uninstall [workflow]",
	Short: "Uninstall Primus Lens components",
	Long: `Uninstall Primus Lens components deployed by the specified workflow.

Examples:
  # Uninstall dataplane
  primus-lens-installer uninstall dataplane

  # Force uninstall (ignore errors)
  primus-lens-installer uninstall dataplane --force

  # Delete persistent data as well
  primus-lens-installer uninstall dataplane --delete-data`,
	Args: cobra.ExactArgs(1),
	RunE: runUninstall,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)

	uninstallCmd.Flags().BoolVar(&force, "force", false, "force uninstall, ignore errors")
	uninstallCmd.Flags().BoolVar(&deleteData, "delete-data", false, "delete persistent volume claims")
	uninstallCmd.Flags().DurationVar(&uninstallTimeout, "timeout", 10*time.Minute, "uninstall timeout")
}

func runUninstall(cmd *cobra.Command, args []string) error {
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

	// Setup context
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt signal, cancelling...")
		cancel()
	}()

	opts := workflow.RunOptions{
		Verbose:    verbose,
		Kubeconfig: getKubeconfig(),
		Namespace:  namespace,
	}

	uninstallOpts := workflow.UninstallOptions{
		Force:      force,
		DeleteData: deleteData,
	}

	fmt.Printf("Starting %s uninstallation...\n", workflowName)
	fmt.Printf("Namespace: %s\n", namespace)
	if force {
		fmt.Println("Mode: FORCE (errors will be ignored)")
	}
	if deleteData {
		fmt.Println("WARNING: Persistent data will be deleted!")
	}
	fmt.Println()

	if err := wf.Uninstall(ctx, opts, uninstallOpts); err != nil {
		return fmt.Errorf("uninstallation failed: %w", err)
	}

	fmt.Println()
	fmt.Printf("✅ %s uninstallation completed!\n", workflowName)
	return nil
}

