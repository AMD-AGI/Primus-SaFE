/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package job

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

type NodeJobInput struct {
	Commands     []NodeJobCommand `json:"commands"`
	DispatchTime int64            `json:"dispatchTime"`
}

type NodeJobCommand struct {
	// the addon name
	Addon string `json:"addon"`
	// the command to be executed by nodeAgent (base64-encoded).
	// Note that only one job command is allowed per node at any given time.
	Action string `json:"action,omitempty"`
	// the command to check the action execution status (base64 encoded).
	// Return 0 on success, 1 on failure.
	Observe string `json:"observe,omitempty"`
	// Determines whether the command should be registered as a service in systemd
	IsSystemd bool `json:"isSystemd,omitempty"`
	// target chip， If left empty, it applies to all chip.
	Chip v1.ChipType `json:"chip,omitempty"`
}

func GetNodeJobInput(obj metav1.Object) *NodeJobInput {
	val := v1.GetNodeJobInput(obj)
	if val == "" {
		return nil
	}
	var jobInput NodeJobInput
	if err := json.Unmarshal([]byte(val), &jobInput); err != nil {
		return nil
	}
	return &jobInput
}
