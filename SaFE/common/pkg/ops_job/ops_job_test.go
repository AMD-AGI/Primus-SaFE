/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func opsJobScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	return s
}

func TestCleanupJobRelatedResource(t *testing.T) {
	labels := map[string]string{v1.OpsJobIdLabel: "job-1"}
	wl := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "wl1", Labels: labels}}
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1", Labels: labels}}
	cl := ctrlfake.NewClientBuilder().WithScheme(opsJobScheme(t)).WithObjects(wl, fault).Build()

	err := CleanupJobRelatedResource(context.Background(), cl, "job-1")
	assert.NoError(t, err)

	// also works when nothing matches
	assert.NoError(t, CleanupJobRelatedResource(context.Background(), cl, "job-none"))
}

func TestGetRequiredParameter(t *testing.T) {
	job := &v1.OpsJob{Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: "p", Value: "v"}}}}
	p, err := GetRequiredParameter(job, "p")
	assert.NoError(t, err)
	assert.Equal(t, "v", p.Value)

	_, err = GetRequiredParameter(job, "missing")
	assert.Error(t, err)
}
