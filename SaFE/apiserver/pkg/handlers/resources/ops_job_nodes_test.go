/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGenerateOpsJobNodesInput(t *testing.T) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
		},
	}
	h, _ := newAdminHandlerWithObjects(node)

	// Node param branch -> resolves workspace from the node.
	job := &v1.OpsJob{
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "node-1"}}},
	}
	isSpecified, err := h.generateOpsJobNodesInput(context.Background(), job)
	assert.NoError(t, err)
	assert.True(t, isSpecified)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))

	// No node scope -> bad request.
	emptyJob := &v1.OpsJob{}
	_, err = h.generateOpsJobNodesInput(context.Background(), emptyJob)
	assert.Error(t, err)

	// Node not found -> error.
	badJob := &v1.OpsJob{
		Spec: v1.OpsJobSpec{Inputs: []v1.Parameter{{Name: v1.ParameterNode, Value: "missing"}}},
	}
	_, err = h.generateOpsJobNodesInput(context.Background(), badJob)
	assert.Error(t, err)
}

func TestGenerateAddonJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
		},
	}
	h, user := newAdminHandlerWithObjects(node)

	body := `{"name":"addon-job","type":"addon","inputs":[{"name":"node","value":"node-1"}],"batchCount":2}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateAddonJob(c, []byte(body))
	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))
}

func TestGeneratePreflightJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.WorkspaceIdLabel: "ws-1"},
		},
	}
	h, user := newAdminHandlerWithObjects(node)

	body := `{"name":"preflight-job","type":"preflight","inputs":[{"name":"node","value":"node-1"}],"image":"repo/img:tag"}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generatePreflightJob(c, []byte(body))
	assert.NoError(t, err)
	assert.NotNil(t, job)
}

func TestGenerateAddonJobNodeNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body := `{"name":"addon-job","type":"addon","inputs":[{"name":"node","value":"missing"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	_, err := h.generateAddonJob(c, []byte(body))
	assert.Error(t, err)
}
