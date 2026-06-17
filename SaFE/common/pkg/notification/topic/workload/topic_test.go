/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
)

func TestTopicName(t *testing.T) {
	assert.Equal(t, model.TopicWorkload, (&Topic{}).Name())
}

func TestFilter(t *testing.T) {
	tp := &Topic{}
	assert.False(t, tp.Filter(map[string]interface{}{}))
	assert.False(t, tp.Filter(map[string]interface{}{"condition": ""}))
	assert.False(t, tp.Filter(map[string]interface{}{"condition": "Pending"}))
	assert.True(t, tp.Filter(map[string]interface{}{"condition": string(v1.WorkloadRunning)}))
	assert.True(t, tp.Filter(map[string]interface{}{"condition": string(v1.WorkloadFailed)}))
}

func TestGetStatusColor(t *testing.T) {
	assert.Equal(t, "#c53030", getStatusColor(string(v1.WorkloadFailed)))
	assert.Equal(t, "#2f855a", getStatusColor(string(v1.WorkloadSucceeded)))
	assert.Equal(t, "#3182ce", getStatusColor(string(v1.WorkloadRunning)))
	assert.Equal(t, "#d69e2e", getStatusColor(string(v1.WorkloadStopped)))
	assert.Equal(t, "#4a5568", getStatusColor("Unknown"))
}

func TestGetWorkloadUrl(t *testing.T) {
	url := getWorkloadUrl("w-123")
	assert.Contains(t, url, "id=w-123")
}

func TestExtractUserEmails(t *testing.T) {
	assert.Empty(t, extractUserEmails(nil))
	assert.Empty(t, extractUserEmails([]*v1.User{{}}))
}

func TestRenderEmailTemplate(t *testing.T) {
	out, err := renderEmailTemplate(EmailData{JobName: "job1", Status: "Failed", StatusColor: "#c53030"})
	assert.NoError(t, err)
	assert.Contains(t, out, "job1")
}

func TestBuildMessageNoRecipients(t *testing.T) {
	tp := &Topic{}
	data := map[string]interface{}{
		"condition": string(v1.WorkloadFailed),
		"message":   "boom",
		"workload":  map[string]interface{}{"metadata": map[string]interface{}{"name": "w1"}},
	}
	// no users with email -> returns nil, nil
	msgs, err := tp.BuildMessage(context.Background(), data)
	assert.NoError(t, err)
	assert.Nil(t, msgs)
}
