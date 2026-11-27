/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"gotest.tools/assert"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockdb "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// memberCloneHandler returns a handler holding the shipped chart roles, a member of redactWorkspace
// with the default role, a user of another workspace, and the stored workload of redactOwnerId.
func memberCloneHandler(t *testing.T) (*Handler, *v1.User, *v1.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("db.enable", "true")
	t.Cleanup(func() { commonconfig.SetValue("db.enable", "") })
	member := redactUser("member-user", v1.DefaultRole)
	outsider := redactUser("outsider-user", v1.DefaultRole)
	commonuser.AssignWorkspace(outsider, "other-ws")
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(append(authtest.ChartRoles(t), member, outsider)...).
		WithScheme(scheme.Scheme).
		Build()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	mockDBClient := mockdb.NewMockInterface(ctrl)
	row := &dbclient.Workload{
		WorkloadId: redactWorkload,
		Workspace:  redactWorkspace,
		Cluster:    "test-cluster",
		Phase:      sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
		UserId:     sql.NullString{String: redactOwnerId, Valid: true},
		GVK:        `{"group":"kubeflow.org","version":"v1","kind":"PyTorchJob"}`,
		Env: sql.NullString{Valid: true, String: fmt.Sprintf(
			`{"ANTHROPIC_API_KEY":%q,"MODE":"train"}`, redactOwnerKey)},
		Secrets: sql.NullString{Valid: true, String: fmt.Sprintf(`[{"id":%q}]`, redactSecretId)},
	}
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), redactWorkload).Return(row, nil).AnyTimes()
	mockDBClient.EXPECT().ListWorkloadPods(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockDBClient.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	h := &Handler{
		Client:           fakeCtrlClient,
		clientSet:        k8sfake.NewSimpleClientset(),
		dbClient:         mockDBClient,
		accessController: &authority.AccessController{Client: fakeCtrlClient},
	}
	return h, member, outsider
}

// getAs calls GET /workloads/{redactWorkload} as caller.
func getAs(h *Handler, caller *v1.User) (interface{}, error) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.UserId, caller.Name)
	c.Set(common.UserName, v1.GetUserName(caller))
	c.Set(common.Name, redactWorkload)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workloads/"+redactWorkload, nil)
	return h.getWorkload(c)
}

// Test_getWorkload_MemberReadsRedactedSpec: under the shipped roles a member of the workload's
// workspace reads another member's workload for cloning, with credential env values masked and
// no secrets; a user of another workspace is refused.
func Test_getWorkload_MemberReadsRedactedSpec(t *testing.T) {
	h, member, outsider := memberCloneHandler(t)

	result, err := getAs(h, member)
	assert.NilError(t, err)
	resp := result.(*view.GetWorkloadResponse)
	assert.Equal(t, resp.Env["ANTHROPIC_API_KEY"], RedactedEnvValue)
	assert.Equal(t, resp.Env["MODE"], "train")
	assert.Equal(t, len(resp.Secrets), 0)

	_, err = getAs(h, outsider)
	assert.ErrorContains(t, err, "not allowed to get")
}

// Test_authWorkloadAction_MemberStillCannotGet: reading the redacted spec does not extend the get
// grant, which also authorizes pod logs, services and SSH/webshell on another member's workload.
func Test_authWorkloadAction_MemberStillCannotGet(t *testing.T) {
	h, member, _ := memberCloneHandler(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.UserId, member.Name)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workloads/"+redactWorkload+"/pods/p/logs", nil)
	w := generateWorkloadForAuth(redactWorkload, redactOwnerId, redactWorkspace, "test-cluster")
	err := h.authWorkloadAction(c, w, v1.GetVerb, v1.WorkloadKind, member, h.accessController.GetRoles(c.Request.Context(), member))
	assert.ErrorContains(t, err, "not allowed to get")
}

// Test_redactedEnvKey: a create request that still carries the mask for an env value is named,
// including under an empty key.
func Test_redactedEnvKey(t *testing.T) {
	_, found := redactedEnvKey(map[string]string{"MODE": "train"})
	assert.Assert(t, !found)
	key, found := redactedEnvKey(map[string]string{"MODE": "train", "API_KEY": RedactedEnvValue})
	assert.Assert(t, found)
	assert.Equal(t, key, "API_KEY")
	_, found = redactedEnvKey(map[string]string{"": RedactedEnvValue, "API_KEY": RedactedEnvValue})
	assert.Assert(t, found)
	_, found = redactedEnvKey(map[string]string{"": RedactedEnvValue})
	assert.Assert(t, found)
	_, found = redactedEnvKey(nil)
	assert.Assert(t, !found)
}

// Test_getWorkload_MemberReadIsNotCountedAsDenied: a member's read through the list grant does not
// count an authorization refusal; a refused read counts one.
func Test_getWorkload_MemberReadIsNotCountedAsDenied(t *testing.T) {
	h, member, outsider := memberCloneHandler(t)
	denied := func() float64 {
		return testutil.ToFloat64(apimetrics.AuthzDeniedTotal.WithLabelValues("workload", string(v1.GetVerb)))
	}

	before := denied()
	_, err := getAs(h, member)
	assert.NilError(t, err)
	assert.Equal(t, denied(), before)

	_, err = getAs(h, outsider)
	assert.ErrorContains(t, err, "not allowed to get")
	assert.Equal(t, denied(), before+1)
}
