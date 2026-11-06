/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
)

const (
	ncWorkspace = "ws-node-change"
	ncNode      = "node-change-1"
	ncPod       = "node-change-1-up"
	ncRole      = "node-operator"

	// ncMember is a member of the node's workspace and is recorded as the node's owner.
	ncMember = "nc-member"
	// ncManager manages the node's workspace.
	ncManager = "nc-manager"
	ncAdmin   = "nc-admin"
	// ncOperator holds the default role and a role that grants node updates on every node.
	ncOperator = "nc-operator"
)

// ncPatch asks for every field a node patch can change.
const ncPatch = `{"taints":[{"key":"maintenance","effect":"NoSchedule"}],"labels":{"team":"b"},` +
	`"privateIP":"10.0.0.99","port":2222}`

// memberNodeGrant is a rule that grants node changes to the members and owners of a resource. The
// default role an earlier chart installed carries the update half of it, and a deployment keeps
// that stored role until the chart is applied again.
var memberNodeGrant = v1.PolicyRule{
	Resources:    []string{"node"},
	GrantedUsers: []string{authority.GrantedWorkspaceUser, authority.GrantedOwner},
	Verbs:        []v1.RoleVerb{v1.AllVerb},
}

type nodeChangeFixture struct {
	h   *Handler
	cli client.Client
}

// newNodeChangeFixture is a cluster with the built-in roles as the chart ships them, a node assigned to
// a workspace with one taint and one label of its own, the pod that provisions it, and users who stand
// in different relations to that workspace. With legacyDefault the stored default role also carries
// memberNodeGrant.
func newNodeChangeFixture(t *testing.T, legacyDefault bool) *nodeChangeFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	roles := authtest.ChartRoles(t)
	if legacyDefault {
		found := false
		for _, o := range roles {
			if r := o.(*v1.Role); r.Name == string(v1.DefaultRole) {
				r.Rules = append(r.Rules, memberNodeGrant)
				found = true
			}
		}
		require.True(t, found, "the chart defines no default role")
	}
	operator := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: ncRole},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"node"},
			GrantedUsers: []string{authority.GrantedAllUser},
			Verbs:        []v1.RoleVerb{v1.GetVerb, v1.ListVerb, v1.UpdateVerb},
		}},
	}
	user := func(name string, workspaces, managed []string, roles ...v1.UserRole) *v1.User {
		u := &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name},
				Annotations: map[string]string{v1.UserNameAnnotation: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{}},
		}
		if len(workspaces) > 0 {
			u.Spec.Resources[common.UserWorkspaces] = workspaces
		}
		if len(managed) > 0 {
			u.Spec.Resources[common.UserManagedWorkspaces] = managed
		}
		return u
	}
	ws := []string{ncWorkspace}
	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{Kind: v1.NodeKind, APIVersion: v1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: ncNode, Labels: map[string]string{
			v1.ClusterIdLabel:   "c1",
			v1.WorkspaceIdLabel: ncWorkspace,
			v1.UserIdLabel:      ncMember,
			"team":              "a",
		}},
		Spec: v1.NodeSpec{
			Cluster:   pointer.String("c1"),
			Workspace: pointer.String(ncWorkspace),
			PrivateIP: "10.0.0.1",
			Port:      pointer.Int32(22),
			Taints: []corev1.Taint{{
				Key: commonfaults.GenerateTaintKey("existing"), Effect: corev1.TaintEffectNoSchedule,
			}},
		},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: ncPod, Namespace: common.PrimusSafeNamespace,
		Labels: map[string]string{
			v1.ClusterManageNodeLabel:   ncNode,
			v1.ClusterManageActionLabel: string(v1.ClusterScaleUpAction),
		},
	}}
	objs := append(roles, operator, node, pod,
		user(ncMember, ws, nil, v1.DefaultRole),
		user(ncManager, ws, ws, v1.DefaultRole),
		user(ncAdmin, nil, nil, v1.SystemAdminRole),
		user(ncOperator, nil, nil, v1.DefaultRole, ncRole),
	)
	cli := fake.NewClientBuilder().WithScheme(getTestScheme()).WithObjects(objs...).Build()
	return &nodeChangeFixture{
		h:   &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}},
		cli: cli,
	}
}

func (f *nodeChangeFixture) ctx(userId, name, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	c.Set(common.Name, name)
	return c
}

func (f *nodeChangeFixture) node(t *testing.T) *v1.Node {
	t.Helper()
	n := &v1.Node{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: ncNode}, n))
	return n
}

func (f *nodeChangeFixture) podExists(t *testing.T) bool {
	t.Helper()
	err := f.cli.Get(t.Context(), client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: ncPod}, &corev1.Pod{})
	if apierrors.IsNotFound(err) {
		return false
	}
	require.NoError(t, err)
	return true
}

// A member or a manager of the workspace a node is assigned to may read the node but not change it:
// not its taints, labels, flavor or address, not its provisioning, not its existence. It holds whether
// the stored default role is the one the chart ships or one that still grants members node changes, and
// for a member the node records as its owner.
func TestWorkspaceMembersCannotChangeNodes(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, who := range []string{ncMember, ncManager} {
			name := who + " with the chart's roles"
			if legacy {
				name = who + " with a default role that grants members node changes"
			}
			t.Run(name, func(t *testing.T) {
				f := newNodeChangeFixture(t, legacy)
				before := f.node(t)

				res, err := f.h.getNode(f.ctx(who, ncNode, ""))
				require.NoError(t, err, "a member reads the nodes of the workspace")
				require.NotNil(t, res)

				_, err = f.h.patchNode(f.ctx(who, ncNode, ncPatch))
				require.True(t, apierrors.IsForbidden(err), "patch: %v", err)
				after := f.node(t)
				require.Equal(t, before.Spec, after.Spec, "the node was changed for a caller who was refused")
				require.Equal(t, before.Labels, after.Labels)
				require.Empty(t, v1.GetNodeLabelAction(after))

				res, err = f.h.retryNodes(f.ctx(who, "", `{"nodeIds":["`+ncNode+`"]}`))
				require.NoError(t, err)
				retry := res.(*view.RetryNodesResponse)
				require.Equal(t, 0, retry.SuccessCount)
				require.Len(t, retry.FailedNodes, 1)
				require.Contains(t, retry.FailedNodes[0].Error, "not allowed")
				require.True(t, f.podExists(t), "the provisioning pod was deleted for a caller who was refused")

				_, err = f.h.deleteNode(f.ctx(who, ncNode, ""))
				require.True(t, apierrors.IsForbidden(err), "delete: %v", err)
				f.node(t)
			})
		}
	}
}

// A system administrator changes the node, and so does a user who holds a role that grants node updates
// on every node: what decides is still the roles, not a fixed list of users.
func TestNodeChangesTakeARoleThatGrantsThemOnEveryNode(t *testing.T) {
	for _, who := range []string{ncAdmin, ncOperator} {
		t.Run(who, func(t *testing.T) {
			f := newNodeChangeFixture(t, false)
			_, err := f.h.patchNode(f.ctx(who, ncNode, ncPatch))
			require.NoError(t, err)
			n := f.node(t)
			require.Len(t, n.Spec.Taints, 1)
			require.Equal(t, commonfaults.GenerateTaintKey("maintenance"), n.Spec.Taints[0].Key)
			require.Equal(t, "b", n.Labels["team"])
			require.Equal(t, "10.0.0.99", n.Spec.PrivateIP)
			require.Equal(t, int32(2222), n.GetSpecPort())

			res, err := f.h.retryNodes(f.ctx(who, "", `{"nodeIds":["`+ncNode+`"]}`))
			require.NoError(t, err)
			require.Equal(t, 1, res.(*view.RetryNodesResponse).SuccessCount)
			require.False(t, f.podExists(t), "retry deletes the provisioning pod")
		})
	}
}

// The roles the chart ships, other than system-admin, read nodes and change none: no rule that covers
// nodes grants a verb other than get and list. Derived from the chart, so a role added later is covered.
func TestBuiltInRolesBelowSystemAdminOnlyReadNodes(t *testing.T) {
	roles := authtest.ChartRoles(t)
	require.NotEmpty(t, roles)
	for _, o := range roles {
		r := o.(*v1.Role)
		if r.Name == string(v1.SystemAdminRole) {
			continue
		}
		for _, rule := range r.Rules {
			covers := false
			for _, res := range rule.Resources {
				if res == "node" || res == authority.AllResource {
					covers = true
				}
			}
			if !covers {
				continue
			}
			for _, verb := range rule.Verbs {
				require.Contains(t, []v1.RoleVerb{v1.GetVerb, v1.ListVerb}, verb,
					"role %s grants %s on nodes to %v", r.Name, verb, rule.GrantedUsers)
			}
		}
	}
}
