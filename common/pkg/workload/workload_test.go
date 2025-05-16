/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestCvtToResourceList(t *testing.T) {
	tests := []struct {
		name     string
		resource v1.WorkloadResource
		gotError bool
	}{
		{
			"success",
			v1.WorkloadResource{
				Replica: 1,
				CPU:     "64",
				Memory:  "100Mi",
				GPU:     "1",
				GPUName: common.AmdGpu,
			},
			false,
		},
		{
			"Invalid cpu",
			v1.WorkloadResource{
				Replica: 1,
				CPU:     "-64",
				Memory:  "100Ki",
			},
			true,
		},
		{
			"Invalid memory",
			v1.WorkloadResource{
				Replica: 1,
				CPU:     "64",
				Memory:  "1000abc",
			},
			true,
		},
		{
			"Invalid gpu",
			v1.WorkloadResource{
				Replica: 2,
				CPU:     "10",
				Memory:  "10Mi",
				GPU:     "-1",
				GPUName: common.AmdGpu,
			},
			true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CvtToResourceList([]v1.WorkloadResource{test.resource})
			assert.Equal(t, err != nil, test.gotError)
		})
	}
}
