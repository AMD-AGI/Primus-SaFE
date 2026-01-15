/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"strings"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"gotest.tools/assert"
)

func TestValidateDisplayName(t *testing.T) {
	tests := []struct {
		name         string
		displayName  string
		workloadKind string
		wantErr      bool
	}{
		{
			name:         "empty name is valid",
			displayName:  "",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "valid simple name",
			displayName:  "prod-29pvc",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "valid name with dots",
			displayName:  "my.app.v1",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "valid name with hyphens",
			displayName:  "my-app-v1",
			workloadKind: common.PytorchJobKind,
			wantErr:      false,
		},
		{
			name:         "valid minimum length name",
			displayName:  "ab",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "invalid - starts with number",
			displayName:  "1abc",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - starts with hyphen",
			displayName:  "-abc",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - ends with hyphen",
			displayName:  "abc-",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - ends with dot",
			displayName:  "abc.",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - uppercase letters",
			displayName:  "MyApp",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - contains underscore",
			displayName:  "my_app",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "invalid - single character",
			displayName:  "a",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "valid max length for deployment",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxDeploymentNameLen-1) + "c",
			workloadKind: common.DeploymentKind,
			wantErr:      false,
		},
		{
			name:         "invalid - exceeds max length for deployment",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxDeploymentNameLen+1) + "c",
			workloadKind: common.DeploymentKind,
			wantErr:      true,
		},
		{
			name:         "valid max length for pytorchjob",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxPytorchJobNameLen-1) + "c",
			workloadKind: common.PytorchJobKind,
			wantErr:      false,
		},
		{
			name:         "invalid - exceeds max length for pytorchjob",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxPytorchJobNameLen+1) + "c",
			workloadKind: common.PytorchJobKind,
			wantErr:      true,
		},
		{
			name:         "valid max length for torchft",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxTorchFTNameLen-1) + "c",
			workloadKind: common.TorchFTKind,
			wantErr:      false,
		},
		{
			name:         "invalid - exceeds max length for torchft",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxTorchFTNameLen+1) + "c",
			workloadKind: common.TorchFTKind,
			wantErr:      true,
		},
		{
			name:         "valid for unknown workload kind uses default length",
			displayName:  "a" + strings.Repeat("b", commonutils.MaxGeneratedNameLength-1) + "c",
			workloadKind: "UnknownKind",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDisplayName(tt.displayName, tt.workloadKind)
			if tt.wantErr {
				assert.Assert(t, err != nil, "expected error but got nil")
				assert.Assert(t, commonerrors.IsBadRequest(err), "expected BadRequest error")
			} else {
				assert.NilError(t, err)
			}
		})
	}
}
