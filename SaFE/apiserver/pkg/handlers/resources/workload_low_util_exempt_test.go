/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// postExempt creates a PyTorchJob through the create route with the given request fields added.
func (f *privilegedTemplateFixture) postExempt(t *testing.T, userId string, extra map[string]any) (*v1.Workload, error) {
	t.Helper()
	fields := map[string]any{
		"displayName":      "ex",
		"workspaceId":      ptWorkspace,
		"images":           []string{"img"},
		"entryPoints":      []string{"sleep"},
		"groupVersionKind": map[string]string{"version": "v1", "kind": common.PytorchJobKind},
		"resources":        []map[string]any{{"replica": 1, "cpu": "1", "memory": "1Gi"}},
	}
	for k, v := range extra {
		fields[k] = v
	}
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	res, err := f.h.createWorkload(f.ginContext(t, userId, http.MethodPost, string(body)))
	if err != nil {
		return nil, err
	}
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: res.(*view.CreateWorkloadResponse).WorkloadId}, stored))
	return stored, nil
}

func (f *privilegedTemplateFixture) patchBody(t *testing.T, userId, name, body string) error {
	t.Helper()
	c := f.ginContext(t, userId, http.MethodPatch, body)
	c.Set(common.Name, name)
	_, err := f.h.patchWorkload(c)
	return err
}

func requireLowUtilExemptRefusal(t *testing.T, err error) {
	t.Helper()
	require.True(t, apierrors.IsForbidden(err), "want 403, got %v", err)
	require.Contains(t, err.Error(), "lowUtilizationExempt")
	require.Contains(t, err.Error(), authority.WorkloadLowUtilExemptKind)
}

func lowUtilExemptEntries(w *v1.Workload) []string {
	var out []string
	for _, cond := range w.Status.Conditions {
		if cond.Type == string(v1.LowUtilizationExempt) {
			out = append(out, cond.Reason+": "+cond.Message)
		}
	}
	return out
}

// Exempting a workload at creation takes the workload/low-util-exempt grant: a member is refused
// with a 403 naming it, the workspace's manager and a system administrator are not, and the
// timeline names who set it. The annotation itself cannot be sent by a request.
func TestLowUtilExemptOnCreate(t *testing.T) {
	f := newPrivilegedTemplateFixture(t)

	_, err := f.postExempt(t, ptMember, map[string]any{"lowUtilizationExempt": true})
	requireLowUtilExemptRefusal(t, err)
	_, err = f.postExempt(t, ptOtherManager, map[string]any{"lowUtilizationExempt": true})
	requireLowUtilExemptRefusal(t, err)
	require.Equal(t, 0, f.count(t), "a refused workload was created")

	stored, err := f.postExempt(t, ptMember, map[string]any{
		"lowUtilizationExempt": false,
		"annotations":          map[string]string{v1.WorkloadLowUtilExemptAnnotation: v1.TrueStr},
	})
	require.NoError(t, err)
	assert.False(t, isLowUtilExempt(stored))
	assert.Empty(t, lowUtilExemptEntries(stored))

	for _, who := range []string{ptManager, ptAdmin} {
		stored, err = f.postExempt(t, who, map[string]any{"lowUtilizationExempt": true})
		require.NoError(t, err, who)
		assert.True(t, isLowUtilExempt(stored), who)
		assert.Equal(t, []string{"Set: exempted from the low utilization policy by " + who}, lowUtilExemptEntries(stored))
	}
}

// Setting or clearing the exemption on a patch takes the grant; a patch that leaves it as it is
// does not. Each change adds a timeline entry naming who made it.
func TestLowUtilExemptOnPatch(t *testing.T) {
	f := newPrivilegedTemplateFixture(t)
	f.stored(t, "job", common.PytorchJobKind, ptMember, nil)
	get := func() *v1.Workload {
		w := &v1.Workload{}
		require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: "job"}, w))
		return w
	}

	requireLowUtilExemptRefusal(t, f.patchBody(t, ptMember, "job", `{"lowUtilizationExempt":true}`))
	assert.False(t, isLowUtilExempt(get()))
	require.NoError(t, f.patchBody(t, ptMember, "job", `{"lowUtilizationExempt":false}`))

	require.NoError(t, f.patchBody(t, ptManager, "job", `{"lowUtilizationExempt":true}`))
	assert.True(t, isLowUtilExempt(get()))
	require.NoError(t, f.patchBody(t, ptMember, "job", `{"description":"changed","lowUtilizationExempt":true}`))
	assert.True(t, isLowUtilExempt(get()))

	requireLowUtilExemptRefusal(t, f.patchBody(t, ptMember, "job", `{"lowUtilizationExempt":false}`))
	assert.True(t, isLowUtilExempt(get()))
	require.NoError(t, f.patchBody(t, ptAdmin, "job", `{"lowUtilizationExempt":false}`))

	stored := get()
	assert.False(t, isLowUtilExempt(stored))
	assert.Equal(t, []string{
		"Set: exempted from the low utilization policy by " + ptManager,
		"Cleared: put back under the low utilization policy by " + ptAdmin,
	}, lowUtilExemptEntries(stored))
	assert.False(t, lowUtilExemptFromConditions(stored.Status.Conditions))
}

// GET reads the exemption from the object of a workload that has not ended, whose timeline in the
// database may lag, and from the timeline of one that has.
func TestLowUtilExemptInGetResponse(t *testing.T) {
	user := genMockUser()
	set, _ := json.Marshal([]metav1.Condition{*newLowUtilExemptCondition(true, "u")})
	dbWorkload := &dbclient.Workload{
		WorkloadId: "w1",
		GVK:        `{"version":"v1","kind":"PyTorchJob"}`,
		Phase:      sql.NullString{String: string(v1.WorkloadSucceeded), Valid: true},
		Conditions: sql.NullString{String: string(set), Valid: true},
	}
	live := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w1"}}
	cli := fake.NewClientBuilder().WithScheme(getTestScheme()).WithObjects(user, live).Build()
	h := &Handler{Client: cli, accessController: authority.NewAccessController(cli)}
	assert.True(t, h.cvtDBWorkloadToGetResponse(t.Context(), user, nil, dbWorkload).LowUtilizationExempt)

	dbWorkload.Phase.String = string(v1.WorkloadRunning)
	assert.False(t, h.cvtDBWorkloadToGetResponse(t.Context(), user, nil, dbWorkload).LowUtilizationExempt)
}

func TestLowUtilExemptFromConditions(t *testing.T) {
	assert.False(t, lowUtilExemptFromConditions(nil))
	set := *newLowUtilExemptCondition(true, "u")
	cleared := *newLowUtilExemptCondition(false, "u")
	other := metav1.Condition{Type: string(v1.RuntimeExtended), Reason: "Set"}
	assert.True(t, lowUtilExemptFromConditions([]metav1.Condition{set, other}))
	assert.False(t, lowUtilExemptFromConditions([]metav1.Condition{set, cleared}))
	assert.True(t, lowUtilExemptFromConditions([]metav1.Condition{cleared, set}))
	for _, cond := range []metav1.Condition{set, cleared} {
		assert.Regexp(t, conditionReasonPattern, cond.Reason)
	}
}
