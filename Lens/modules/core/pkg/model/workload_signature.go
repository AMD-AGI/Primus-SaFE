// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

// WorkloadSignature represents the signature of a workload for similarity calculation
type WorkloadSignature struct {
	// Basic features
	Image     string            `json:"image"`     // Image name
	Command   []string          `json:"command"`   // Startup command
	Args      []string          `json:"args"`      // Command arguments
	Env       map[string]string `json:"env"`       // Environment variables
	Labels    map[string]string `json:"labels"`    // Labels
	Namespace string            `json:"namespace"` // Namespace

	// Fast matching hashes
	ImageHash   string `json:"image_hash"`   // MD5(image)
	CommandHash string `json:"command_hash"` // MD5(sorted_command)
	EnvHash     string `json:"env_hash"`     // MD5(sorted_key_env_vars)
}

