/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	assert.True(t, tp.Filter(map[string]interface{}{"condition": ConditionLowUtilization}))
}

func TestGetStatusColor(t *testing.T) {
	assert.Equal(t, "#c53030", getStatusColor(string(v1.WorkloadFailed)))
	assert.Equal(t, "#2f855a", getStatusColor(string(v1.WorkloadSucceeded)))
	assert.Equal(t, "#3182ce", getStatusColor(string(v1.WorkloadRunning)))
	assert.Equal(t, "#d69e2e", getStatusColor(string(v1.WorkloadStopped)))
	assert.Equal(t, "#dd6b20", getStatusColor(ConditionLowUtilization))
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

func notifiedUser() *v1.User {
	return &v1.User{ObjectMeta: metav1.ObjectMeta{
		Name:        "u1",
		Annotations: map[string]string{v1.UserEmailAnnotation: "u1@example.com"},
	}}
}

func TestBuildMessageLowUtilization(t *testing.T) {
	data := map[string]interface{}{
		"condition": ConditionLowUtilization,
		"workload":  &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}},
		"users":     []*v1.User{notifiedUser()},
		"lowUtilization": &LowUtilizationNotice{
			Threshold: 5, Window: "1h", StopTime: "2026-01-02 12:30:00 UTC",
		},
	}
	msgs, err := (&Topic{}).BuildMessage(context.Background(), data)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	assert.Equal(t, []string{"u1@example.com"}, msgs[0].Email.To)
	assert.Contains(t, msgs[0].Email.Title, ConditionLowUtilization)
	content := msgs[0].Email.Content
	assert.Contains(t, content, "stayed below the workspace threshold")
	assert.Contains(t, content, "5%")
	assert.Contains(t, content, "1h")
	assert.Contains(t, content, "2026-01-02 12:30:00 UTC")
}

func TestBuildMessageLowUtilizationRules(t *testing.T) {
	data := map[string]interface{}{
		"condition": ConditionLowUtilization,
		"workload":  &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}},
		"users":     []*v1.User{notifiedUser()},
		"lowUtilization": &LowUtilizationNotice{
			StopTime: "2026-01-02 12:30:00 UTC",
			Rules: []string{
				"low GPU utilization below 11% for 30m: mode Average, observed max 3.0%",
				"low GPU utilization below 30% for 1h: mode Average, observed max 25.0%",
			},
		},
	}
	msgs, err := (&Topic{}).BuildMessage(context.Background(), data)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	content := msgs[0].Email.Content
	assert.Contains(t, content, "low GPU utilization below 11% for 30m: mode Average, observed max 3.0%<br>"+
		"low GPU utilization below 30% for 1h: mode Average, observed max 25.0%")
	assert.NotContains(t, content, "GPU Threshold")
	assert.Contains(t, content, "2026-01-02 12:30:00 UTC")
}

func TestBuildMessageStoppedShowsReason(t *testing.T) {
	data := map[string]interface{}{
		"condition": string(v1.WorkloadStopped),
		"message":   "stopped for low GPU utilization",
		"workload":  &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}},
		"users":     []*v1.User{notifiedUser()},
	}
	msgs, err := (&Topic{}).BuildMessage(context.Background(), data)
	assert.NoError(t, err)
	assert.Len(t, msgs, 1)
	content := msgs[0].Email.Content
	assert.Contains(t, content, "Reason")
	assert.Contains(t, content, "stopped for low GPU utilization")
	assert.NotContains(t, content, "Planned Stop")
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
