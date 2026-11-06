/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

const (
	authzWorkspace = "opt-authz-ws"
	authzOtherWS   = "opt-authz-other-ws"
	authzTaskID    = "opt-authz-task"
	// authzOwner submitted the task and is a member of authzWorkspace.
	authzOwner = "opt-authz-owner"
	// authzMember is another member of authzWorkspace.
	authzMember = "opt-authz-member"
	// authzManager manages authzWorkspace and is a member of no workspace.
	authzManager = "opt-authz-manager"
	// authzOutsider is a member of authzOtherWS only.
	authzOutsider = "opt-authz-outsider"
	// authzAdmin is a system administrator in no workspace.
	authzAdmin = "opt-authz-admin"
	// authzReader is a read-only system administrator in no workspace.
	authzReader = "opt-authz-reader"
	// authzNoAccess holds only the self-read role, though it is a member of authzWorkspace: being a
	// member grants nothing the roles do not.
	authzNoAccess = "opt-authz-self-read"
)

var authzUsers = []string{
	authzOwner, authzMember, authzManager, authzOutsider, authzAdmin, authzReader, authzNoAccess,
}

// authzAccessController is an access controller over the built-in roles as the chart ships them and
// the users above.
func authzAccessController(t *testing.T) *authority.AccessController {
	t.Helper()
	user := func(name string, roles []v1.UserRole, workspaces, managed []string) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Resources: map[string][]string{
				common.UserWorkspaces: workspaces, common.UserManagedWorkspaces: managed}},
		}
	}
	def := []v1.UserRole{v1.DefaultRole}
	objs := append(authtest.ChartRoles(t),
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: authzWorkspace}},
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: authzOtherWS}},
		user(authzOwner, def, []string{authzWorkspace}, nil),
		user(authzMember, def, []string{authzWorkspace}, nil),
		user(authzManager, def, nil, []string{authzWorkspace}),
		user(authzOutsider, def, []string{authzOtherWS}, nil),
		user(authzAdmin, []v1.UserRole{v1.SystemAdminRole}, nil, nil),
		user(authzReader, []v1.UserRole{v1.SystemAdminReadonlyRole}, nil, nil),
		user(authzNoAccess, []v1.UserRole{v1.SelfReadRole}, []string{authzWorkspace}, nil),
	)
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	return &authority.AccessController{Client: fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()}
}

// authzTaskHandler is a handler whose database holds one failed task, submitted by authzOwner in
// authzWorkspace, and whose Claw counts the requests it is sent.
func authzTaskHandler(t *testing.T) (*Handler, *mock_client.MockInterface, *atomic.Int32) {
	t.Helper()
	var clawCalls atomic.Int32
	claw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		clawCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(claw.Close)
	mockDB := mock_client.NewMockInterface(gomock.NewController(t))
	mockDB.EXPECT().GetOptimizationTask(gomock.Any(), authzTaskID).Return(&dbclient.OptimizationTask{
		ID: authzTaskID, DisplayName: "opt", Workspace: authzWorkspace, UserID: authzOwner, UserName: authzOwner,
		ModelID: "model-1", ClawSessionID: "session-1", Status: dbclient.OptimizationTaskStatusFailed,
	}, nil).AnyTimes()
	return &Handler{
		dbClient:   mockDB,
		k8sClient:  fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build(),
		accessCtrl: authzAccessController(t),
		clawClient: NewClawClient(claw.URL, ""),
		hubs:       newHubRegistry(),
	}, mockDB, &clawCalls
}

// callAs calls a task route for authzTaskID as userId and returns the response status.
func callAs(t *testing.T, route func(*Handler, *gin.Context), h *Handler, userId, target string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/"+target, nil)
	c.Params = gin.Params{{Key: "id", Value: authzTaskID}}
	c.Set(common.UserId, userId)
	c.Set(common.UserName, userId)
	route(h, c)
	return c.Writer.Status(), rec.Body.String()
}

type taskRoute struct {
	name   string
	call   func(*Handler, *gin.Context)
	target string
	verb   v1.RoleVerb
	// allowed are the users the route serves; every other user is refused with 403.
	allowed []string
	// expect arms the database calls a served request makes.
	expect func(*mock_client.MockInterface)
	// served checks the response to a served request.
	served func(t *testing.T, code int, body string, clawCalls int32)
}

var (
	taskReaders = []string{authzOwner, authzMember, authzManager, authzAdmin, authzReader}
	taskActors  = []string{authzOwner, authzManager, authzAdmin}
)

func taskRoutes() []taskRoute {
	noDB := func(*mock_client.MockInterface) {}
	return []taskRoute{{
		name: "get", call: (*Handler).GetTask, verb: v1.GetVerb, allowed: taskReaders, expect: noDB,
		served: func(t *testing.T, code int, body string, _ int32) {
			require.Equal(t, http.StatusOK, code, body)
			require.Contains(t, body, authzTaskID)
		},
	}, {
		name: "events", call: (*Handler).StreamEvents, verb: v1.GetVerb, allowed: taskReaders,
		expect: func(db *mock_client.MockInterface) {
			db.EXPECT().ListOptimizationEvents(gomock.Any(), authzTaskID, int64(0), gomock.Any()).Return(nil, nil)
		},
		served: func(t *testing.T, code int, body string, _ int32) {
			require.Equal(t, http.StatusOK, code, body)
			require.Contains(t, body, "event: done")
		},
	}, {
		name: "artifacts", call: (*Handler).ListArtifacts, verb: v1.GetVerb, allowed: taskReaders, expect: noDB,
		served: func(t *testing.T, code int, body string, clawCalls int32) {
			require.Equal(t, http.StatusOK, code, body)
			require.Positive(t, clawCalls)
		},
	}, {
		name: "download", call: (*Handler).DownloadArtifact, target: "?path=out.txt", verb: v1.GetVerb,
		allowed: taskReaders, expect: noDB,
		served: func(t *testing.T, code int, body string, clawCalls int32) {
			require.Equal(t, http.StatusOK, code, body)
			require.Positive(t, clawCalls)
		},
	}, {
		// Reading the task is all this change checks for apply; the model lookup that follows
		// fails here, which shows the request got past the check.
		name: "apply", call: (*Handler).ApplyTask, verb: v1.GetVerb, allowed: taskReaders,
		expect: func(db *mock_client.MockInterface) {
			db.EXPECT().GetModelByID(gomock.Any(), "model-1").Return(nil, errors.New("model lookup reached"))
		},
		served: func(t *testing.T, code int, body string, _ int32) {
			require.Equal(t, http.StatusBadRequest, code, body)
			require.Contains(t, body, "model lookup reached")
		},
	}, {
		name: "interrupt", call: (*Handler).InterruptTask, verb: v1.DeleteVerb, allowed: taskActors,
		expect: func(db *mock_client.MockInterface) {
			db.EXPECT().UpdateOptimizationTaskStatus(gomock.Any(), authzTaskID,
				dbclient.OptimizationTaskStatusInterrupted, gomock.Any(), gomock.Any()).Return(nil)
		},
		served: func(t *testing.T, code int, body string, clawCalls int32) {
			require.Equal(t, http.StatusOK, code, body)
			require.Positive(t, clawCalls)
		},
	}, {
		// The retried task is submitted as the caller; with no Claw credential the submission
		// stops at its first step, which shows the request got past the check.
		name: "retry", call: (*Handler).RetryTask, verb: v1.ResumeVerb, allowed: taskActors, expect: noDB,
		served: func(t *testing.T, code int, body string, _ int32) {
			require.Equal(t, http.StatusUnauthorized, code, body)
			require.Contains(t, body, "PrimusClaw authentication required")
		},
	}, {
		name: "delete", call: (*Handler).DeleteTask, verb: v1.DeleteVerb, allowed: taskActors,
		expect: func(db *mock_client.MockInterface) {
			db.EXPECT().DeleteOptimizationTask(gomock.Any(), authzTaskID).Return(nil)
		},
		served: func(t *testing.T, code int, body string, clawCalls int32) {
			require.Equal(t, http.StatusNoContent, code, body)
			require.Positive(t, clawCalls)
		},
	}}
}

// Each task route checks the caller against the task as against the workload it runs, owned by
// the task's submitter in the task's workspace: the submitter, the workspace's manager and a system
// administrator may read and act on it; another member of the workspace and a read-only system
// administrator may read it only; a member of another workspace and a member of the task's workspace
// whose roles grant nothing (self-read only) may do neither. A refused request is 403 and reaches neither the database (beyond loading the task) nor
// Claw.
func TestTaskRoutesAuthorizeLikeTheTasksWorkload(t *testing.T) {
	for _, route := range taskRoutes() {
		allowed := map[string]bool{}
		for _, u := range route.allowed {
			allowed[u] = true
		}
		for _, user := range authzUsers {
			t.Run(route.name+"/"+user, func(t *testing.T) {
				h, db, clawCalls := authzTaskHandler(t)
				if allowed[user] {
					route.expect(db)
				}
				code, body := callAs(t, route.call, h, user, route.target)
				if allowed[user] {
					route.served(t, code, body, clawCalls.Load())
					return
				}
				require.Equal(t, http.StatusForbidden, code, body)
				require.Contains(t, body, "not allowed to "+string(route.verb)+" workload")
				require.Zero(t, clawCalls.Load(), "a refused request reached Claw")
			})
		}
	}
}

// Without an access controller every task route refuses, and does nothing.
func TestTaskRoutesFailClosedWithoutAccessController(t *testing.T) {
	for _, route := range taskRoutes() {
		t.Run(route.name, func(t *testing.T) {
			h, _, clawCalls := authzTaskHandler(t)
			h.accessCtrl = nil
			code, body := callAs(t, route.call, h, authzOwner, route.target)
			require.Equal(t, http.StatusInternalServerError, code, body)
			require.Contains(t, body, accessControllerMissing)
			require.Zero(t, clawCalls.Load())
		})
	}
}

// The task list holds what the caller may see: every task for a system administrator and a
// read-only one; otherwise the caller's own tasks and those of each workspace the caller belongs to
// or manages in which the roles grant it; nothing for a member whose roles grant nothing. The
// caller's own filters still apply, and the narrowing is not reported as authorization denials.
func TestListTasksHoldsWhatTheCallerMaySee(t *testing.T) {
	for _, tc := range []struct {
		user string
		want *dbclient.OptimizationTaskScope
	}{
		{authzAdmin, nil},
		{authzReader, nil},
		{authzOwner, &dbclient.OptimizationTaskScope{UserID: authzOwner, Workspaces: []string{authzWorkspace}}},
		{authzMember, &dbclient.OptimizationTaskScope{UserID: authzMember, Workspaces: []string{authzWorkspace}}},
		{authzManager, &dbclient.OptimizationTaskScope{UserID: authzManager, Workspaces: []string{authzWorkspace}}},
		{authzOutsider, &dbclient.OptimizationTaskScope{UserID: authzOutsider, Workspaces: []string{authzOtherWS}}},
		{authzNoAccess, &dbclient.OptimizationTaskScope{}},
	} {
		t.Run(tc.user, func(t *testing.T) {
			h, db, _ := authzTaskHandler(t)
			var got dbclient.OptimizationTaskFilter
			db.EXPECT().ListOptimizationTasks(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ interface{}, f dbclient.OptimizationTaskFilter) ([]*dbclient.OptimizationTask, int64, error) {
					got = f
					return nil, 0, nil
				})
			denied := apimetrics.AuthzDeniedTotal.WithLabelValues("workload", string(v1.ListVerb))
			before := testutil.ToFloat64(denied)
			code, body := callAs(t, (*Handler).ListTasks, h, tc.user, "?status=Failed&workspace="+authzWorkspace)
			require.Equal(t, http.StatusOK, code, body)
			require.Equal(t, before, testutil.ToFloat64(denied), "narrowing the list was counted as a refusal")
			require.Equal(t, "Failed", got.Status)
			require.Equal(t, authzWorkspace, got.Workspace)
			if got.Scope != nil {
				sort.Strings(got.Scope.Workspaces)
			}
			require.Equal(t, tc.want, got.Scope)
		})
	}
}

// The list refuses an unregistered caller and a handler without an access controller, without
// reading the database.
func TestListTasksRefusesWithoutAUser(t *testing.T) {
	h, _, _ := authzTaskHandler(t)
	code, body := callAs(t, (*Handler).ListTasks, h, "opt-authz-unregistered", "")
	require.Equal(t, http.StatusUnauthorized, code, body)

	h.accessCtrl = nil
	code, body = callAs(t, (*Handler).ListTasks, h, authzOwner, "")
	require.Equal(t, http.StatusInternalServerError, code, body)
}
