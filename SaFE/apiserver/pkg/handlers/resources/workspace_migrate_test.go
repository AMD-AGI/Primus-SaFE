/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func newProcessNodesContext(t *testing.T, workspaceId, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	c.Set(common.UserId, "test-user")
	c.Set(common.Name, workspaceId)
	return c
}

// A migration needs somewhere to go, and the other actions have nowhere to put a target --
// silently ignoring one would accept a request that does not do what it says.
func TestParseProcessNodesRequestMigrateTarget(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "migrate with a target",
			body: `{"nodeIds":["node1"],"action":"migrate","targetWorkspaceId":"ws-b"}`,
		},
		{
			name:    "migrate without a target",
			body:    `{"nodeIds":["node1"],"action":"migrate"}`,
			wantErr: true,
		},
		{
			name:    "add with a target",
			body:    `{"nodeIds":["node1"],"action":"add","targetWorkspaceId":"ws-b"}`,
			wantErr: true,
		},
		{
			name: "add without a target",
			body: `{"nodeIds":["node1"],"action":"add"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseProcessNodesRequest(newProcessNodesContext(t, "ws-a", tc.body))
			if tc.wantErr {
				testifyassert.Error(t, err)
				return
			}
			testifyassert.NoError(t, err)
		})
	}
}

// The REST layer's whole job here is to turn the request into the annotation the webhook and
// the reconciler read. If it writes a bare "migrate", nothing downstream knows where the node
// is meant to go.
func TestProcessWorkspaceNodesWritesTheMigrateAction(t *testing.T) {
	source := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-a"}}
	target := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	h, _ := newAdminHandlerWithObjects(source, target)

	c := newProcessNodesContext(t, "ws-a",
		`{"nodeIds":["node1"],"action":"migrate","targetWorkspaceId":"ws-b"}`)
	_, err := h.processWorkspaceNodes(c)
	testifyassert.NoError(t, err)

	stored := &v1.Workspace{}
	testifyassert.NoError(t, h.Get(c.Request.Context(), client.ObjectKey{Name: "ws-a"}, stored))
	testifyassert.JSONEq(t, `{"node1":"migrate:ws-b"}`, v1.GetWorkspaceNodesAction(stored))
	// The target is only told about the nodes once the source has released them.
	testifyassert.NoError(t, h.Get(c.Request.Context(), client.ObjectKey{Name: "ws-b"}, target))
	testifyassert.Empty(t, v1.GetWorkspaceNodesAction(target))
}

// The route names the source, so the update authorizes the source and nothing else. A
// workspace manager who may move nodes out of their own workspace must not, by that alone,
// be able to push them into someone else's.
func TestProcessWorkspaceNodesAuthorizesTheTarget(t *testing.T) {
	manager := &v1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ws-a-manager",
			Labels: map[string]string{v1.UserIdLabel: "ws-a-manager"},
		},
		Spec: v1.UserSpec{
			Type:      v1.DefaultUserType,
			Resources: map[string][]string{common.UserManagedWorkspaces: {"ws-a"}},
		},
	}
	workspaceAdmin := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.WorkspaceAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedWorkspaceUser},
		}},
	}
	source := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-a"}}
	target := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}}
	// Built by hand rather than through newAdminHandlerWithObjects: the access controller
	// behind that helper is a process-wide singleton bound to whichever client reached it
	// first, so a test that brings its own users has to hold its own controller.
	scheme := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(scheme))
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(manager, workspaceAdmin, source, target).Build()
	h := &Handler{Client: fakeClient, accessController: &authority.AccessController{Client: fakeClient}}

	// Their own workspace is theirs to change: a plain removal goes through.
	allowed := newProcessNodesContext(t, "ws-a", `{"nodeIds":["node1"],"action":"remove"}`)
	allowed.Set(common.UserId, manager.Name)
	_, err := h.processWorkspaceNodes(allowed)
	testifyassert.NoError(t, err)

	c := newProcessNodesContext(t, "ws-a",
		`{"nodeIds":["node1"],"action":"migrate","targetWorkspaceId":"ws-b"}`)
	c.Set(common.UserId, manager.Name)
	_, err = h.processWorkspaceNodes(c)
	testifyassert.Error(t, err)

	stored := &v1.Workspace{}
	testifyassert.NoError(t, h.Get(c.Request.Context(), client.ObjectKey{Name: "ws-a"}, stored))
	testifyassert.NotContains(t, v1.GetWorkspaceNodesAction(stored), v1.NodeActionMigrate)
}
