/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestGenerateDumpLogJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withS3(t)

	wl := newWorkloadForLog("wl-1", "c1", "ws-1")
	h, user := newAdminHandlerWithObjects(wl)

	body := `{"name":"dumplog","type":"dumplog","inputs":[{"name":"workload","value":"wl-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateDumpLogJob(c, []byte(body))
	assert.NoError(t, err)
	assert.Equal(t, "wl-1", job.Name)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))

	// Missing workload parameter -> bad request.
	body2 := `{"name":"dumplog","type":"dumplog","inputs":[{"name":"foo","value":"bar"}]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generateDumpLogJob(c2, []byte(body2))
	assert.Error(t, err)
}

func TestGenerateDownloadJob(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Handler with workspace CR (ctrl client) + a general secret (clientSet).
	user := genMockUser()
	role := genMockRole()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-1"}, Spec: v1.WorkspaceSpec{Cluster: "c1"}}
	sch := runtime.NewScheme()
	_ = v1.AddToScheme(sch)
	ctrlClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(user, role, ws).Build()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec-1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral)},
		},
	}
	h := &Handler{
		Client:           ctrlClient,
		clientSet:        k8sfake.NewSimpleClientset(secret),
		accessController: authority.NewAccessController(ctrlClient),
	}

	body := `{"name":"download","type":"download","inputs":[{"name":"secret","value":"sec-1"},{"name":"workspace","value":"ws-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateDownloadJob(c, []byte(body))
	assert.NoError(t, err)
	assert.Equal(t, "ws-1", v1.GetWorkspaceId(job))
	assert.Equal(t, "c1", v1.GetClusterId(job))

	// Missing secret param -> bad request.
	body2 := `{"name":"download","type":"download","inputs":[{"name":"workspace","value":"ws-1"}]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generateDownloadJob(c2, []byte(body2))
	assert.Error(t, err)
}
