package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/workflow"
)

// statusCmd represents the status command
var statusCmd = &cobra.Command{
	Use:   "status [workflow]",
	Short: "Check the status of Primus Lens components",
	Long: `Check the deployment status of Primus Lens components.

Examples:
  # Check dataplane status
  primus-lens-installer status dataplane

  # Check all components
  primus-lens-installer status all`,
	Args: cobra.ExactArgs(1),
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	opts := workflow.RunOptions{
		Verbose:    verbose,
		Kubeconfig: getKubeconfig(),
		Namespace:  namespace,
	}

	fmt.Printf("Checking %s status...\n", workflowName)
	fmt.Printf("Namespace: %s\n\n", namespace)

	status, err := wf.Status(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	// Print status
	printStatus(status)

	return nil
}

func printStatus(status *workflow.Status) {
	fmt.Println("Stage Status:")
	fmt.Println("─────────────────────────────────────────")

	for _, stage := range status.Stages {
		icon := getStatusIcon(stage.State)
		fmt.Printf("%s %-25s %s\n", icon, stage.Name, stage.State)

		if verbose && stage.Message != "" {
			fmt.Printf("   └─ %s\n", stage.Message)
		}

		for _, resource := range stage.Resources {
			resIcon := getStatusIcon(resource.State)
			fmt.Printf("   %s %s/%s\n", resIcon, resource.Kind, resource.Name)
		}
	}

	fmt.Println("─────────────────────────────────────────")
	fmt.Printf("Overall: %s %s\n", getStatusIcon(status.OverallState), status.OverallState)
}

func getStatusIcon(state workflow.State) string {
	switch state {
	case workflow.StateReady:
		return "✅"
	case workflow.StateInProgress:
		return "🔄"
	case workflow.StatePending:
		return "⏳"
	case workflow.StateFailed:
		return "❌"
	case workflow.StateUnknown:
		return "❓"
	default:
		return "•"
	}
}
