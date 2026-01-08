// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stage

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/types"
)

// Options contains common options for stages
type Options struct {
	Namespace  string
	Kubeconfig string
	DryRun     bool
	Verbose    bool
}

// FromRunOptions converts types.RunOptions to stage.Options
func FromRunOptions(opts types.RunOptions) Options {
	return Options{
		Namespace:  opts.Namespace,
		Kubeconfig: opts.Kubeconfig,
		DryRun:     opts.DryRun,
		Verbose:    opts.Verbose,
	}
}

// Executor defines the interface for executing commands
type Executor interface {
	// Helm executes helm commands
	Helm(ctx context.Context, args ...string) (string, error)

	// Kubectl executes kubectl commands
	Kubectl(ctx context.Context, args ...string) (string, error)
}
