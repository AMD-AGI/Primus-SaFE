/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"testing"

	testifyassert "github.com/stretchr/testify/assert"
)

// The job-manager reads external_execution.user_dir_mounts from the config the chart renders: off
// unless the operator turns it on, so external Pods keep mounting a hostpath volume whole.
func TestChartRendersExternalUserDirMounts(t *testing.T) {
	render := func(values ...string) string {
		return renderConfigMapData(t, "job-manager", "config.yaml", values...)
	}

	loadRendered(t, render())
	testifyassert.False(t, IsExternalUserDirMountsEnable())

	loadRendered(t, render("--set", "external_execution.user_dir_mounts=true"))
	testifyassert.True(t, IsExternalUserDirMountsEnable())
	// The other external execution settings are rendered next to it and are not disturbed.
	testifyassert.False(t, IsExternalExecutionEnable())

	loadRendered(t, render("--set", "external_execution.enabled=true", "--set", "external_execution.user_dir_mounts=false"))
	testifyassert.False(t, IsExternalUserDirMountsEnable())
	testifyassert.True(t, IsExternalExecutionEnable())
}
