/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"testing"
	"time"

	"gotest.tools/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func TestValidateCronJobs(t *testing.T) {
	nowTime := time.Now().UTC()
	tests := []struct {
		name   string
		t      time.Time
		result bool
	}{
		{"Past time", nowTime.Add(-time.Hour), false},
		{"Future 1 minute", nowTime.Add(time.Minute), true},
		{"Future 6 months", nowTime.AddDate(0, 6, 0), true},
		{"Almost 1 year but less 1 minute", nowTime.AddDate(1, 0, 0).Add(-time.Minute), true},
		{"Exactly 1 year", nowTime.AddDate(1, 0, 0), false},
		{"Over 1 year", nowTime.AddDate(1, 0, 0).Add(time.Minute), false},
		{"now", nowTime, false},
	}

	var validator WorkloadValidator
	for _, tt := range tests {
		workload := &v1.Workload{
			Spec: v1.WorkloadSpec{
				CronJobs: []v1.CronJob{{
					Schedule: tt.t.Format(timeutil.TimeRFC3339Milli),
					Action:   v1.CronStart,
				}},
			},
		}
		err := validator.validateCronJobs(workload)
		assert.Equal(t, tt.result, err == nil)
	}
}
