/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePreflightReport(t *testing.T) {
	input := `2026-03-13 15:05:38
[GPU6] Step [11/12] Loss: 7.6475 LR: 5.00e-05 Grad Norm: 3.1557 ETA: 0:06
2026-03-13 15:05:38
[GPU6] Training ended at step 12 [GPU 6]
================================================================================
                    PrimusBench Node Check Report
================================================================================
Generated at: 2026-03-13 01:49:10
================================================================================

Failed Nodes (Node Check) - 2 nodes
--------------------------------------------------------------------------------
  uswslocpm2m-106-1792 (10.158.173.117): [uswslocpm2m-106-1792] [NODE-53] [NODE] [ERROR]: ERROR: Could not find a version that satisfies the requirement tensorboard (from versions: none)ERROR: No matching distribution found for tensorboard[2026-03-13 01:19:12] ERROR: Failed to install dependencies 
  uswslocpm2m-106-1647 (10.158.162.130): [babel_stream_memory.sh] failed to clone babel_stream  

Failed Nodes (Network Check) - 2 nodes
--------------------------------------------------------------------------------
  uswslocpm2m-106-1177 (10.158.160.198)
  uswslocpm2m-106-1909 (10.158.175.187)

Healthy Nodes (Passed All Checks) - 2 nodes
--------------------------------------------------------------------------------
  uswslocpm2m-106-1625 (10.158.160.255)
  uswslocpm2m-106-1724 (10.158.160.237)

================================================================================

Summary: 2 healthy nodes out of 6 total nodes checked

================================================================================`

	report := parsePreflightReport([]byte(input))
	assert.NotNil(t, report, "parsePreflightReport should return non-nil when report format is found")

	expectedFailed := []string{
		"uswslocpm2m-106-1792",
		"uswslocpm2m-106-1647",
		"uswslocpm2m-106-1177",
		"uswslocpm2m-106-1909",
	}
	expectedHealthy := []string{
		"uswslocpm2m-106-1625",
		"uswslocpm2m-106-1724",
	}

	assert.Equal(t, expectedFailed, report.FailedNodes, "FailedNodes should match expected")
	assert.Equal(t, expectedHealthy, report.HealthyNodes, "HealthyNodes should match expected")
}

func TestParsePreflightReport_NoReport(t *testing.T) {
	input := `2026-03-13 15:05:38
[GPU6] Step [11/12] Loss: 7.6475 LR: 5.00e-05
Some random log without PrimusBench report`
	report := parsePreflightReport([]byte(input))
	assert.Nil(t, report, "parsePreflightReport should return nil when report format is not found")
}
