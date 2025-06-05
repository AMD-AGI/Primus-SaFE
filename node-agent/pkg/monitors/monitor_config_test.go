/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

func TestValidate(t *testing.T) {
	conf := &MonitorConfig{
		Id:            "001",
		Script:        "test.sh",
		Cronjob:       "@every 30s",
		TimeoutSecond: 25,
		Chip:          types.AmdGpuChip,
		Toggle:        "on",
	}
	conf.SetDefaults()
	assert.Equal(t, conf.ConsecutiveCount, 1)

	err := conf.Validate()
	assert.Nil(t, err)

}
