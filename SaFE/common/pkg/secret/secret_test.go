/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func secretWithAnnotation(val string) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s"}}
	if val != "" {
		s.Annotations = map[string]string{v1.WorkspaceIdsAnnotation: val}
	}
	return s
}

func TestGetSecretWorkspaces(t *testing.T) {
	// no annotation -> nil
	assert.Nil(t, GetSecretWorkspaces(secretWithAnnotation("")))
	// valid json array
	assert.Equal(t, []string{"ws1", "ws2"}, GetSecretWorkspaces(secretWithAnnotation(`["ws1","ws2"]`)))
	// invalid json -> nil
	assert.Nil(t, GetSecretWorkspaces(secretWithAnnotation(`not-json`)))
}
