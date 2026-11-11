/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlruntimefake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
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
	redactImages    = `["rocm/pytorch:latest","rocm/vllm:latest"]`
	redactDBURL     = "postgres://app:owner-db-pass@db:5432/app"
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

// Test_getWorkload_EnvVisibleOnlyToOwnerAndSystemAdmin pins who sees a workload's credential
// env values and secrets through GET /workloads/{id}: the owner and system administrators
// see the stored values; a workspace member and a holder of a role granted to all users
// ("*") can get the workload but see credential values masked and no secrets. Plain
// configuration values are returned to every caller.
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
					`{"ANTHROPIC_API_KEY":%q,"CLAW_SESSION_ID":%q,"IMAGES":%q,"UNIFIED_JOB_ENABLE":"true","DATABASE_URL":%q}`,
					redactOwnerKey, redactOwnerSID, redactImages, redactDBURL)},
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
			assert.Equal(t, len(resp.Env), 5, tt.description)
			// Plain configuration is returned as stored to every caller: the Web UI parses it
			// (JSON.parse(IMAGES), UNIFIED_JOB_ENABLE === "true").
			assert.Equal(t, resp.Env["CLAW_SESSION_ID"], redactOwnerSID, tt.description)
			assert.Equal(t, resp.Env["IMAGES"], redactImages, tt.description)
			assert.Equal(t, resp.Env["UNIFIED_JOB_ENABLE"], "true", tt.description)
			if tt.wantValues {
				assert.Equal(t, resp.Env["ANTHROPIC_API_KEY"], redactOwnerKey, tt.description)
				assert.Equal(t, resp.Env["DATABASE_URL"], redactDBURL, tt.description)
				assert.Equal(t, len(resp.Secrets), 1, tt.description)
				assert.Equal(t, resp.Secrets[0].Id, redactSecretId, tt.description)
			} else {
				assert.Equal(t, resp.Env["ANTHROPIC_API_KEY"], RedactedEnvValue, tt.description)
				assert.Equal(t, resp.Env["DATABASE_URL"], RedactedEnvValue, tt.description)
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

// Test_redactEnvValues_MasksOnlyCredentials: a variable is masked when its name names a
// credential (the audit log's judgement) or its value is a URL with a password in it.
func Test_redactEnvValues_MasksOnlyCredentials(t *testing.T) {
	out := redactEnvValues(map[string]string{
		"ANTHROPIC_API_KEY": "k", "AUTH_TOKEN": "t", "LANGFUSE_SECRET_KEY": "s", "DB_PASSWORD": "p",
		"DB_CREDENTIALS": "c", "DATABASE_URL": redactDBURL, "CACHE_URL": "redis://:pw@cache:6379/0", "PROXY_URL": "http://proxy:3128",
		"IMAGES": redactImages, "REPLICA_COUNT": "2", "UNIFIED_JOB_ENABLE": "true",
	})
	for _, k := range []string{"ANTHROPIC_API_KEY", "AUTH_TOKEN", "LANGFUSE_SECRET_KEY", "DB_PASSWORD", "DB_CREDENTIALS", "DATABASE_URL", "CACHE_URL"} {
		assert.Equal(t, out[k], RedactedEnvValue, k)
	}
	assert.Equal(t, out["PROXY_URL"], "http://proxy:3128")
	assert.Equal(t, out["IMAGES"], redactImages)
	assert.Equal(t, out["REPLICA_COUNT"], "2")
	assert.Equal(t, out["UNIFIED_JOB_ENABLE"], "true")
	assert.Equal(t, len(out), 11)
}

// Test_resumeWorkload_MaskedEnvKeepsStoredValues: a caller who may resume another user's
// workload but not see its credentials (a workspace administrator) sends back what GET gave
// it: credential env values masked and no secrets. The resumed workload keeps the stored
// values; values the caller changed are taken as sent; secrets are only those it names.
func Test_resumeWorkload_MaskedEnvKeepsStoredValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("db.enable", "true")
	defer commonconfig.SetValue("db.enable", "")
	ownerSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: redactSecretId, Namespace: common.PrimusSafeNamespace,
		Labels: map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral)},
	}}
	h, caller := userIdHandler(t)
	h.clientSet = k8sfake.NewSimpleClientset(ownerSecret)
	assert.Assert(t, caller.Name != redactOwnerId)
	assert.Assert(t, !caller.IsSystemAdmin())

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDBClient := mockdb.NewMockInterface(ctrl)
	row := &dbclient.Workload{
		WorkloadId: redactWorkload,
		Workspace:  "test-workspace",
		Cluster:    "test-cluster",
		Phase:      sql.NullString{String: string(v1.WorkloadStopped), Valid: true},
		UserId:     sql.NullString{String: redactOwnerId, Valid: true},
		UserName:   sql.NullString{String: redactOwnerId, Valid: true},
		Env: sql.NullString{Valid: true, String: fmt.Sprintf(
			`{"ANTHROPIC_API_KEY":%q,"DATABASE_URL":%q,"MODE":"old"}`, redactOwnerKey, redactDBURL)},
		Secrets: sql.NullString{Valid: true, String: fmt.Sprintf(`[{"id":%q}]`, redactSecretId)},
	}
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), redactWorkload).Return(row, nil).AnyTimes()
	mockDBClient.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockDBClient.EXPECT().ArchiveWorkloadNodesForResume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	h.dbClient = mockDBClient

	body := fmt.Sprintf(`{
		"displayName": "resumed",
		"workspaceId": "test-workspace",
		"images": ["pytorch/pytorch:latest"],
		"entryPoints": ["python train.py"],
		"groupVersionKind": {"version": "v1", "kind": "PyTorchJob"},
		"resources": [{"replica": 1, "cpu": "16", "gpu": "4", "memory": "64Gi"}],
		"env": {"ANTHROPIC_API_KEY": %q, "DATABASE_URL": %q, "MODE": "new"}
	}`, RedactedEnvValue, RedactedEnvValue)
	c := userIdRequest(caller, http.MethodPost, "/api/v1/workloads/"+redactWorkload+"/resume", body)
	c.Set(common.Name, redactWorkload)
	_, err := h.resumeWorkload(c)
	assert.NilError(t, err)

	resumed := &v1.Workload{}
	assert.NilError(t, h.Get(context.Background(), client.ObjectKey{Name: redactWorkload}, resumed))
	assert.Equal(t, v1.GetUserId(resumed), redactOwnerId)
	assert.Equal(t, resumed.Spec.Env["ANTHROPIC_API_KEY"], redactOwnerKey)
	assert.Equal(t, resumed.Spec.Env["DATABASE_URL"], redactDBURL)
	assert.Equal(t, resumed.Spec.Env["MODE"], "new")
	assert.Equal(t, len(resumed.Spec.Secrets), 0)
}

// resumeAsWorkspaceAdmin resumes the stored workload of redactOwnerId as a workspace
// administrator of its workspace who holds the shipped default and workspace-admin roles,
// which grant every verb on workloads there but only list on secrets: the caller cannot get
// the owner's secret.
func resumeAsWorkspaceAdmin(t *testing.T, secretsField string) (*Handler, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("db.enable", "true")
	t.Cleanup(func() { commonconfig.SetValue("db.enable", "") })
	const workspace = "test-workspace"
	caller := genMockUser()
	caller.Name = "ws-admin"
	caller.Labels[v1.UserIdLabel] = caller.Name
	caller.Spec.Roles = []v1.UserRole{v1.DefaultRole}
	commonuser.AssignWorkspace(caller, workspace)
	commonuser.AssignManagedWorkspace(caller, workspace)
	controlPlaneNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane-node-1",
			Labels: map[string]string{common.KubernetesControlPlane: ""}},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeInternalIP, Address: "192.168.1.100"}}},
	}
	mockScheme := scheme.Scheme
	_ = corev1.AddToScheme(mockScheme)
	_ = v1.AddToScheme(mockScheme)
	fakeCtrlClient := ctrlruntimefake.NewClientBuilder().
		WithObjects(append(authtest.ChartRoles(t), caller, controlPlaneNode)...).
		WithScheme(mockScheme).
		WithStatusSubresource(&v1.Workload{}).
		Build()
	ownerSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: redactSecretId, Namespace: common.PrimusSafeNamespace,
		Labels: map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral), v1.UserIdLabel: redactOwnerId},
	}}
	h := &Handler{
		Client:           fakeCtrlClient,
		clientSet:        k8sfake.NewSimpleClientset(ownerSecret),
		accessController: &authority.AccessController{Client: fakeCtrlClient},
	}

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)
	mockDBClient := mockdb.NewMockInterface(ctrl)
	row := &dbclient.Workload{
		WorkloadId: redactWorkload,
		Workspace:  workspace,
		Cluster:    "test-cluster",
		Phase:      sql.NullString{String: string(v1.WorkloadStopped), Valid: true},
		UserId:     sql.NullString{String: redactOwnerId, Valid: true},
		UserName:   sql.NullString{String: redactOwnerId, Valid: true},
		Env:        sql.NullString{Valid: true, String: fmt.Sprintf(`{"ANTHROPIC_API_KEY":%q}`, redactOwnerKey)},
		Secrets:    sql.NullString{Valid: true, String: fmt.Sprintf(`[{"id":%q}]`, redactSecretId)},
	}
	mockDBClient.EXPECT().GetWorkload(gomock.Any(), redactWorkload).Return(row, nil).AnyTimes()
	mockDBClient.EXPECT().ListWorkloadDispatchNodes(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockDBClient.EXPECT().ArchiveWorkloadNodesForResume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	h.dbClient = mockDBClient

	body := fmt.Sprintf(`{
		"displayName": "resumed",
		"workspaceId": %q,
		"images": ["pytorch/pytorch:latest"],
		"entryPoints": ["python train.py"],
		"groupVersionKind": {"version": "v1", "kind": "PyTorchJob"},
		"resources": [{"replica": 1, "cpu": "16", "gpu": "4", "memory": "64Gi"}],
		"env": {"ANTHROPIC_API_KEY": %q}%s
	}`, workspace, RedactedEnvValue, secretsField)
	c := userIdRequest(caller, http.MethodPost, "/api/v1/workloads/"+redactWorkload+"/resume", body)
	c.Set(common.Name, redactWorkload)
	_, err := h.resumeWorkload(c)
	return h, err
}

// Test_resumeWorkload_WorkspaceAdminWithoutSecretGet: a workspace administrator who may not
// get the owner's secret resumes the owner's workload with what GET gave it (no secrets).
// The resume succeeds, as on main, without the owner's secrets: they are not put back for a
// caller who could write the rest of the spec (image, entry point) and so run its own code
// with them. A secret the caller names itself is authorized against the caller and refused.
func Test_resumeWorkload_WorkspaceAdminWithoutSecretGet(t *testing.T) {
	t.Run("no secrets field: resumed without the owner's secrets", func(t *testing.T) {
		h, err := resumeAsWorkspaceAdmin(t, "")
		assert.NilError(t, err)
		resumed := &v1.Workload{}
		assert.NilError(t, h.Get(context.Background(), client.ObjectKey{Name: redactWorkload}, resumed))
		assert.Equal(t, v1.GetUserId(resumed), redactOwnerId)
		assert.Equal(t, resumed.Spec.Env["ANTHROPIC_API_KEY"], redactOwnerKey)
		assert.Equal(t, len(resumed.Spec.Secrets), 0)
	})
	t.Run("caller names the secret: authorized against the caller", func(t *testing.T) {
		h, err := resumeAsWorkspaceAdmin(t, fmt.Sprintf(`, "secrets": [{"id": %q}]`, redactSecretId))
		assert.ErrorContains(t, err, "not allowed to get")
		resumed := &v1.Workload{}
		assert.Assert(t, h.Get(context.Background(), client.ObjectKey{Name: redactWorkload}, resumed) != nil)
	})
	t.Run("explicit empty secrets: resumed without secrets", func(t *testing.T) {
		h, err := resumeAsWorkspaceAdmin(t, `, "secrets": []`)
		assert.NilError(t, err)
		resumed := &v1.Workload{}
		assert.NilError(t, h.Get(context.Background(), client.ObjectKey{Name: redactWorkload}, resumed))
		assert.Equal(t, len(resumed.Spec.Secrets), 0)
	})
}
