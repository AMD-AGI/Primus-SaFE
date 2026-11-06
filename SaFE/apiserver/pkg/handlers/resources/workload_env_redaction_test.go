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
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mockdb "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

const (
	redactWorkspace = "shared-ws"
	redactWorkload  = "sandbox-redact"
	redactOwnerId   = "owner-user"
	redactOwnerKey  = "owner-credential-value"
	redactOwnerSID  = "session-1234"
	redactSecretId  = "owner-secret-1"
)

func redactUser(name string, roles ...v1.UserRole) *v1.User {
	u := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Labels:      map[string]string{v1.UserIdLabel: name},
			Annotations: map[string]string{v1.UserNameAnnotation: name},
		},
		Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
	}
	commonuser.AssignWorkspace(u, redactWorkspace)
	return u
}

func redactRole(name string, rules ...v1.PolicyRule) *v1.Role {
	return &v1.Role{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: rules}
}

// Test_getWorkload_EnvVisibleOnlyToOwnerAndSystemAdmin pins who sees a workload's env
// values and secrets through GET /workloads/{id}: the owner and system administrators
// see the stored values; a workspace member and a holder of a role granted to all users
// ("*") can get the workload but see only the env keys, with masked values, and no secrets.
func Test_getWorkload_EnvVisibleOnlyToOwnerAndSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")
	defaultRole := redactRole(string(v1.DefaultRole),
		v1.PolicyRule{GrantedUsers: []string{authority.GrantedOwner},
			Resources: []string{"workload"}, Verbs: []v1.RoleVerb{v1.GetVerb, v1.ListVerb}},
		v1.PolicyRule{GrantedUsers: []string{"workspace-user"},
			Resources: []string{"workload"}, Verbs: []v1.RoleVerb{v1.CreateVerb, v1.GetVerb, v1.ListVerb}})
	allUsersReader := redactRole("workload-reader",
		v1.PolicyRule{GrantedUsers: []string{authority.GrantedAllUser},
			Resources: []string{"workload"}, Verbs: []v1.RoleVerb{v1.GetVerb, v1.ListVerb}})
	adminRole := redactRole(string(v1.SystemAdminRole),
		v1.PolicyRule{GrantedUsers: []string{authority.GrantedAllUser},
			Resources: []string{authority.AllResource}, Verbs: []v1.RoleVerb{v1.AllVerb}})
	readonlyAdminRole := redactRole(string(v1.SystemAdminReadonlyRole),
		v1.PolicyRule{GrantedUsers: []string{authority.GrantedAllUser},
			Resources: []string{authority.AllResource}, Verbs: []v1.RoleVerb{v1.GetVerb, v1.ListVerb}})

	owner := redactUser(redactOwnerId, v1.DefaultRole)
	member := redactUser("member-user", v1.DefaultRole)
	reader := redactUser("reader-user", v1.DefaultRole, "workload-reader")
	commonuser.AssignWorkspace(reader) // no workspace: only the "*" role lets it read
	admin := redactUser("admin-user", v1.SystemAdminRole)
	readonlyAdmin := redactUser("readonly-admin-user", v1.SystemAdminReadonlyRole)

	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(defaultRole, allUsersReader, adminRole, readonlyAdminRole,
			owner, member, reader, admin, readonlyAdmin).
		WithScheme(scheme.Scheme).
		Build()

	tests := []struct {
		name        string
		caller      *v1.User
		wantValues  bool
		description string
	}{
		{"owner", owner, true, "the owner sees its own env"},
		{"system admin", admin, true, "a system administrator sees env"},
		{"read-only system admin", readonlyAdmin, true, "a read-only system administrator sees env"},
		{"workspace member", member, false, "a member of the workload's workspace sees masked values"},
		{"role granted to all users", reader, false, "a role with grantedUsers * sees masked values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockDBClient := mockdb.NewMockInterface(ctrl)
			row := &dbclient.Workload{
				WorkloadId: redactWorkload,
				Workspace:  redactWorkspace,
				Cluster:    "test-cluster",
				Phase:      sql.NullString{String: string(v1.WorkloadRunning), Valid: true},
				UserId:     sql.NullString{String: redactOwnerId, Valid: true},
				GVK:        `{"group":"amd.com","version":"v1","kind":"Sandbox"}`,
				Env: sql.NullString{Valid: true, String: fmt.Sprintf(
					`{"ANTHROPIC_API_KEY":%q,"CLAW_SESSION_ID":%q}`, redactOwnerKey, redactOwnerSID)},
				Secrets: sql.NullString{Valid: true, String: fmt.Sprintf(`[{"id":%q}]`, redactSecretId)},
			}
			mockDBClient.EXPECT().GetWorkload(gomock.Any(), redactWorkload).Return(row, nil).AnyTimes()
			mockDBClient.EXPECT().ListWorkloadPods(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			mockDBClient.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			h := Handler{
				Client:           fakeCtrlClient,
				clientSet:        k8sfake.NewSimpleClientset(),
				dbClient:         mockDBClient,
				accessController: &authority.AccessController{Client: fakeCtrlClient},
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(common.UserId, tt.caller.Name)
			c.Set(common.UserName, v1.GetUserName(tt.caller))
			c.Set(common.Name, redactWorkload)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workloads/"+redactWorkload, nil)

			result, err := h.getWorkload(c)
			assert.NilError(t, err, tt.description)
			resp := result.(*view.GetWorkloadResponse)
			assert.Equal(t, len(resp.Env), 2, tt.description)
			if tt.wantValues {
				assert.Equal(t, resp.Env["ANTHROPIC_API_KEY"], redactOwnerKey, tt.description)
				assert.Equal(t, resp.Env["CLAW_SESSION_ID"], redactOwnerSID, tt.description)
				assert.Equal(t, len(resp.Secrets), 1, tt.description)
				assert.Equal(t, resp.Secrets[0].Id, redactSecretId, tt.description)
			} else {
				assert.Equal(t, resp.Env["ANTHROPIC_API_KEY"], RedactedEnvValue, tt.description)
				assert.Equal(t, resp.Env["CLAW_SESSION_ID"], RedactedEnvValue, tt.description)
				assert.Equal(t, len(resp.Secrets), 0, tt.description)
			}
		})
	}
}

// Test_applyWorkloadPatch_MaskedEnvKeepsStoredValue: a caller who saw masked env and
// sends it back unchanged must not overwrite the stored values with the mask.
func Test_applyWorkloadPatch_MaskedEnvKeepsStoredValue(t *testing.T) {
	w := &v1.Workload{Spec: v1.WorkloadSpec{Env: map[string]string{
		"ANTHROPIC_API_KEY": redactOwnerKey, "MODE": "old"}}}
	env := map[string]string{"ANTHROPIC_API_KEY": RedactedEnvValue, "MODE": "new", "NEW_KEY": RedactedEnvValue}
	assert.NilError(t, applyWorkloadPatch(w, &view.PatchWorkloadRequest{Env: &env}))
	assert.Equal(t, w.Spec.Env["ANTHROPIC_API_KEY"], redactOwnerKey)
	assert.Equal(t, w.Spec.Env["MODE"], "new")
	// A key the workload never had is stored as sent: there is nothing to keep.
	assert.Equal(t, w.Spec.Env["NEW_KEY"], RedactedEnvValue)
	assert.Equal(t, len(w.Spec.Env), 3)
}
