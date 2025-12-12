package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile    string
	namespace  string
	kubeconfig string
	verbose    bool
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "primus-lens-installer",
	Short: "Primus Lens Installer - A multi-stage deployment tool for Primus Lens",
	Long: `Primus Lens Installer is a CLI tool that orchestrates the deployment
of Primus Lens components in multiple stages, ensuring each stage completes
successfully before proceeding to the next.

Supported workflows:
  - dataplane:    Deploy data plane components (operators, infrastructure, apps)
  - controlplane: Deploy control plane components (API, Adapter)
  - standalone:   Deploy all components in a single cluster

Example:
  primus-lens-installer install dataplane --config values.yaml
  primus-lens-installer status dataplane
  primus-lens-installer uninstall dataplane`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is ./values.yaml)")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "primus-lens", "kubernetes namespace")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
}

func getKubeconfig() string {
	// If explicitly set via flag, use it
	if kubeconfig != "" {
		return kubeconfig
	}
	// If set via environment variable, use it
	if env := os.Getenv("KUBECONFIG"); env != "" {
		return env
	}
	// Check if default kubeconfig exists
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	defaultPath := fmt.Sprintf("%s/.kube/config", home)
	// Only return the path if the file exists
	// Otherwise return empty string to use in-cluster config
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}
	return ""
}
