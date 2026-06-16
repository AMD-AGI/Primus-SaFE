/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestParseMetricsJSON(t *testing.T) {
	arr, err := parseMetricsJSON([]byte(`[{"a":1},{"b":2}]`))
	assert.NoError(t, err)
	assert.Len(t, arr, 2)

	obj, err := parseMetricsJSON([]byte(`{"a":1}`))
	assert.NoError(t, err)
	assert.Len(t, obj, 1)

	_, err = parseMetricsJSON([]byte(`"scalar"`))
	assert.Error(t, err)

	_, err = parseMetricsJSON([]byte(`not-json`))
	assert.Error(t, err)
}

func TestMatchesAnyPattern(t *testing.T) {
	assert.True(t, matchesAnyPattern("/a/b/file.json", nil))
	assert.True(t, matchesAnyPattern("/a/b/metrics.json", []string{"**/metrics.json"}))
	assert.True(t, matchesAnyPattern("/a/b/c.json", []string{"/a/b/c.json"}))
	assert.False(t, matchesAnyPattern("/a/b/c.json", []string{"*.csv"}))
}

func TestInt64Ptr(t *testing.T) {
	p := int64Ptr(42)
	assert.Equal(t, int64(42), *p)
}

func TestWaitForPodRunningSuccess(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	cs := k8sfake.NewSimpleClientset(pod)
	err := waitForPodRunning(context.Background(), cs, "ns", "p", 5*time.Second)
	assert.NoError(t, err)
}

func TestWaitForPodRunningFailedPhase(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodFailed},
	}
	cs := k8sfake.NewSimpleClientset(pod)
	err := waitForPodRunning(context.Background(), cs, "ns", "p", 5*time.Second)
	assert.Error(t, err)
}

func TestWaitForPodRunningTimeout(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Status:     corev1.PodStatus{Phase: corev1.PodPending},
	}
	cs := k8sfake.NewSimpleClientset(pod)
	err := waitForPodRunning(context.Background(), cs, "ns", "p", 3*time.Second)
	assert.Error(t, err)
}
