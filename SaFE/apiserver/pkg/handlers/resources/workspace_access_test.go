/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbmodel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// fakeAccessDB keeps workspace access requests and notifications in memory. Other database
// methods are not implemented.
type fakeAccessDB struct {
	dbclient.Interface
	mu            sync.Mutex
	nextId        int64
	reqs          map[int64]*dbclient.WorkspaceAccessRequest
	notifications []*dbmodel.Notification
	// revertFailures is the number of revert calls that fail before one succeeds.
	revertFailures int
	revertCalls    int
}

// newFakeAccessDB returns an empty fake database.
func newFakeAccessDB() *fakeAccessDB {
	return &fakeAccessDB{reqs: map[int64]*dbclient.WorkspaceAccessRequest{}}
}

// CreateWorkspaceAccessRequest stores a copy of the request and sets its id, unless the user already has a
// pending request for the workspace.
func (f *fakeAccessDB) CreateWorkspaceAccessRequest(_ context.Context, req *dbclient.WorkspaceAccessRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if other := f.pendingOf(req.Workspace, req.UserID, 0); other != 0 {
		return &dbclient.PendingAccessRequestError{ID: other}
	}
	f.insert(req)
	return nil
}

// RevertWorkspaceAccessApproval moves an approved request back to pending, or to cancelled when the user
// has another pending request for the workspace.
func (f *fakeAccessDB) RevertWorkspaceAccessApproval(_ context.Context, id int64) (*dbclient.WorkspaceAccessRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revertCalls++
	if f.revertCalls <= f.revertFailures {
		return nil, errors.New("database unavailable")
	}
	req, ok := f.reqs[id]
	if !ok || req.Status != dbclient.WorkspaceAccessApproved {
		return nil, gorm.ErrRecordNotFound
	}
	req.Status, req.DecidedBy, req.DecisionComment = dbclient.WorkspaceAccessPending, "", ""
	if f.pendingOf(req.Workspace, req.UserID, id) != 0 {
		req.Status = dbclient.WorkspaceAccessCancelled
	}
	copied := *req
	return &copied, nil
}

// pendingOf returns the id of a pending request of the user for the workspace other than except, or 0.
func (f *fakeAccessDB) pendingOf(workspace, userId string, except int64) int64 {
	for id, r := range f.reqs {
		if id != except && r.Workspace == workspace && r.UserID == userId && r.Status == dbclient.WorkspaceAccessPending {
			return id
		}
	}
	return 0
}

// insert stores a copy of the request and sets its id.
func (f *fakeAccessDB) insert(req *dbclient.WorkspaceAccessRequest) {
	f.nextId++
	req.ID = f.nextId
	copied := *req
	f.reqs[req.ID] = &copied
}

// GetWorkspaceAccessRequest returns a copy of the request or gorm.ErrRecordNotFound.
func (f *fakeAccessDB) GetWorkspaceAccessRequest(_ context.Context, id int64) (*dbclient.WorkspaceAccessRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	req, ok := f.reqs[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *req
	return &copied, nil
}

// ListWorkspaceAccessRequests returns the matching requests, newest first.
func (f *fakeAccessDB) ListWorkspaceAccessRequests(_ context.Context,
	filter dbclient.WorkspaceAccessRequestFilter) ([]*dbclient.WorkspaceAccessRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []*dbclient.WorkspaceAccessRequest
	for id := f.nextId; id > 0; id-- {
		req, ok := f.reqs[id]
		if !ok {
			continue
		}
		if filter.UserID != "" && req.UserID != filter.UserID ||
			filter.Workspace != "" && req.Workspace != filter.Workspace ||
			filter.Status != "" && req.Status != filter.Status {
			continue
		}
		if filter.Workspaces != nil && !containsString(filter.Workspaces, req.Workspace) {
			continue
		}
		copied := *req
		result = append(result, &copied)
	}
	return result, nil
}

// TransitionWorkspaceAccessRequest applies the transition when the request is in the From state.
func (f *fakeAccessDB) TransitionWorkspaceAccessRequest(_ context.Context, id int64,
	t dbclient.WorkspaceAccessTransition) (*dbclient.WorkspaceAccessRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	req, ok := f.reqs[id]
	if !ok || req.Status != t.From {
		return nil, gorm.ErrRecordNotFound
	}
	req.Status, req.DecidedBy, req.DecisionComment = t.To, t.DecidedBy, t.Comment
	copied := *req
	return &copied, nil
}

// SubmitNotification records the notification.
func (f *fakeAccessDB) SubmitNotification(_ context.Context, data *dbmodel.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notifications = append(f.notifications, data)
	return nil
}

// put stores a request as given, without the pending check, and returns its id.
func (f *fakeAccessDB) put(workspace, userId, status string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.insert(&dbclient.WorkspaceAccessRequest{
		Workspace: workspace, UserID: userId, UserName: userId, Status: status, CreatedAt: time.Now()})
	return f.nextId
}

// recipients returns the recipients of the notifications of the event.
func (f *fakeAccessDB) recipients(event string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var to []string
	for _, n := range f.notifications {
		if n.Data["event"] != event {
			continue
		}
		for _, r := range n.Data["to"].([]interface{}) {
			to = append(to, r.(string))
		}
	}
	return to
}

// containsString reports whether the list holds the value.
func containsString(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// accessRequestFixture is a cluster with an approval workspace ws-open (managed by mgr), a closed workspace
// ws-closed, a member of ws-open, an outsider, a requester and a system administrator.
type accessRequestFixture struct {
	h  *Handler
	db *fakeAccessDB
}

// newAccessRequestFixture builds the handler over the fixture objects and a fake database, with notifications on.
func newAccessRequestFixture(t *testing.T, extra ...runtime.Object) *accessRequestFixture {
	gin.SetMode(gin.TestMode)
	viper.Set("notification.enable", true)
	viper.Set("global.domain", "example.com")
	viper.Set("global.sub_domain", "safe")
	t.Cleanup(viper.Reset)
	open := genWorkspaceFixture("ws-open", "Open WS")
	open.Spec.AccessPolicy = v1.WorkspaceAccessApproval
	open.Spec.Managers = []string{"mgr"}
	open.Spec.Teams = []v1.WorkspaceTeam{{Name: "t1"}}
	closed := genWorkspaceFixture("ws-closed", "Closed WS")
	other := genWorkspaceFixture("ws-other", "Other WS")
	other.Spec.AccessPolicy = v1.WorkspaceAccessApproval
	objs := []runtime.Object{open, closed, other,
		withEmail(genUserFixture("mgr", []v1.UserRole{v1.DefaultRole}, nil, []string{"ws-open"})),
		withEmail(genUserFixture("member", []v1.UserRole{v1.DefaultRole}, []string{"ws-open"}, nil)),
		withEmail(genUserFixture("alice", []v1.UserRole{v1.DefaultRole}, nil, nil)),
		withEmail(genUserFixture("bob", nil, nil, nil)),
		withEmail(genUserFixture("admin", []v1.UserRole{v1.SystemAdminRole}, nil, nil)),
	}
	objs = append(objs, extra...)
	db := newFakeAccessDB()
	h := newTeamsHandler(objs...)
	h.dbClient = db
	return &accessRequestFixture{h: h, db: db}
}

// withEmail sets the e-mail of the user to <id>@example.com.
func withEmail(user *v1.User) *v1.User {
	v1.SetAnnotation(user, v1.UserEmailAnnotation, user.Name+"@example.com")
	return user
}

// call runs the handler as the user with the route parameter, query and JSON body.
func (f *accessRequestFixture) call(fn gin.HandlerFunc, method, userId, name, query, body string,
	headers ...string) *httptest.ResponseRecorder {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(method, "/?"+query, strings.NewReader(body))
	for i := 0; i+1 < len(headers); i += 2 {
		c.Request.Header.Set(headers[i], headers[i+1])
	}
	c.Set(common.UserId, userId)
	c.Set(common.Name, name)
	fn(c)
	return rsp
}

// callIn runs the handler as call does, on a request whose context is ctx.
func (f *accessRequestFixture) callIn(ctx context.Context, fn gin.HandlerFunc, method, userId, name, query,
	body string) *httptest.ResponseRecorder {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(method, "/?"+query, strings.NewReader(body)).WithContext(ctx)
	c.Set(common.UserId, userId)
	c.Set(common.Name, name)
	fn(c)
	return rsp
}

// create submits a request for the workspace as the user.
func (f *accessRequestFixture) create(userId, workspace string) *httptest.ResponseRecorder {
	return f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, userId, "", "",
		`{"workspace":"`+workspace+`","reason":"need gpus"}`)
}

// decodeItem decodes a request item from the response.
func decodeItem(t *testing.T, rsp *httptest.ResponseRecorder) view.WorkspaceAccessRequestDetail {
	var item view.WorkspaceAccessRequestDetail
	require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &item), rsp.Body.String())
	return item
}

// A request is accepted only for an approval workspace the caller does not hold yet, once per workspace while
// pending, with a valid body; the approvers are notified of it.
func TestCreateWorkspaceAccessRequestRules(t *testing.T) {
	f := newAccessRequestFixture(t)

	closed := f.create("alice", "ws-closed")
	require.Equal(t, http.StatusForbidden, closed.Code, closed.Body.String())
	missing := f.create("alice", "ws-missing")
	require.Equal(t, http.StatusForbidden, missing.Code, missing.Body.String())
	require.Equal(t, closed.Body.String(), missing.Body.String(), "a missing workspace answers as a closed one")

	require.Equal(t, http.StatusConflict, f.create("member", "ws-open").Code)
	require.Equal(t, http.StatusConflict, f.create("mgr", "ws-open").Code)
	require.Equal(t, http.StatusConflict, f.create("admin", "ws-open").Code)
	require.Equal(t, http.StatusBadRequest, f.create("alice", "").Code)
	require.Equal(t, http.StatusBadRequest, f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "alice", "", "",
		`{"workspace":"ws-open","reason":"`+strings.Repeat("x", maxAccessReasonLength+1)+`"}`).Code)

	rsp := f.create("alice", "ws-open")
	require.Equal(t, http.StatusCreated, rsp.Code, rsp.Body.String())
	item := decodeItem(t, rsp)
	require.Equal(t, dbclient.WorkspaceAccessPending, item.Status)
	require.Equal(t, "Open WS", item.WorkspaceName)
	require.Equal(t, "alice@example.com", item.UserEmail)
	require.Equal(t, "need gpus", item.Reason)
	require.Equal(t, "https://safe.example.com/workspace-access/requests/"+strconv.FormatInt(item.Id, 10), item.ApprovalUrl)
	require.Equal(t, []string{"mgr@example.com"}, f.db.recipients("created"))

	dup := f.create("alice", "ws-open")
	require.Equal(t, http.StatusConflict, dup.Code)
	require.Contains(t, dup.Body.String(), "pending")
}

// Without a manager the new request is sent to the system administrators.
func TestCreateWorkspaceAccessRequestNotifiesAdminsWithoutManagers(t *testing.T) {
	f := newAccessRequestFixture(t)
	require.Equal(t, http.StatusCreated, f.create("alice", "ws-other").Code)
	require.Equal(t, []string{"admin@example.com"}, f.db.recipients("created"))
}

// A browser request from another site is refused.
func TestWorkspaceAccessRequestRequiresSameOrigin(t *testing.T) {
	f := newAccessRequestFixture(t)
	body := `{"workspace":"ws-open"}`
	require.Equal(t, http.StatusForbidden, f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "alice", "", "", body,
		"Origin", "https://evil.example.org").Code)
	require.Equal(t, http.StatusForbidden, f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "alice", "", "", body,
		"Sec-Fetch-Site", "cross-site").Code)
	require.Equal(t, http.StatusCreated, f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "alice", "", "", body,
		"Origin", "https://safe.example.com").Code)
}

// The Origin must match the console origin or the request's own scheme and host in full; the forwarded host
// is not trusted, and an opaque origin is refused.
func TestSameOriginComparesTheFullOrigin(t *testing.T) {
	f := newAccessRequestFixture(t)
	body := `{"workspace":"ws-open"}`
	refused := [][]string{
		{"Origin", "null"},
		{"Origin", "http://safe.example.com"},
		{"Origin", "https://example.com"},
		{"Origin", "https://evil.example.org", "X-Forwarded-Host", "evil.example.org"},
	}
	for _, headers := range refused {
		rsp := f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "alice", "", "", body, headers...)
		require.Equal(t, http.StatusForbidden, rsp.Code, "%v: %s", headers, rsp.Body.String())
	}
	rsp := f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "alice", "", "", body, "Origin", "http://example.com")
	require.Equal(t, http.StatusCreated, rsp.Code, rsp.Body.String())
	rsp = f.call(f.h.CreateWorkspaceAccessRequest, http.MethodPost, "bob", "", "", body,
		"Origin", "https://example.com", "X-Forwarded-Proto", "https")
	require.Equal(t, http.StatusCreated, rsp.Code, rsp.Body.String())
}

// The detail answers the requester and the approvers; anyone else, and a missing id, get the same 403.
func TestGetWorkspaceAccessRequestVisibility(t *testing.T) {
	f := newAccessRequestFixture(t)
	id := strconv.FormatInt(f.db.put("ws-open", "alice", dbclient.WorkspaceAccessPending), 10)

	mine := f.call(f.h.GetWorkspaceAccessRequest, http.MethodGet, "alice", id, "", "")
	require.Equal(t, http.StatusOK, mine.Code, mine.Body.String())
	item := decodeItem(t, mine)
	require.True(t, item.IsRequester)
	require.False(t, item.CanDecide)

	for _, approver := range []string{"mgr", "admin"} {
		rsp := f.call(f.h.GetWorkspaceAccessRequest, http.MethodGet, approver, id, "", "")
		require.Equal(t, http.StatusOK, rsp.Code, approver)
		item = decodeItem(t, rsp)
		require.True(t, item.CanDecide, approver)
		require.False(t, item.IsRequester, approver)
	}

	outsider := f.call(f.h.GetWorkspaceAccessRequest, http.MethodGet, "bob", id, "", "")
	require.Equal(t, http.StatusForbidden, outsider.Code)
	member := f.call(f.h.GetWorkspaceAccessRequest, http.MethodGet, "member", id, "", "")
	require.Equal(t, http.StatusForbidden, member.Code)
	missing := f.call(f.h.GetWorkspaceAccessRequest, http.MethodGet, "bob", "999", "", "")
	invalid := f.call(f.h.GetWorkspaceAccessRequest, http.MethodGet, "bob", "abc", "", "")
	require.Equal(t, outsider.Body.String(), missing.Body.String())
	require.Equal(t, outsider.Body.String(), invalid.Body.String())
	require.NotContains(t, outsider.Body.String(), "alice")
}

// An approval writes the workspace and the default role to the requester, once.
func TestApproveWorkspaceAccessRequest(t *testing.T) {
	f := newAccessRequestFixture(t)
	id := strconv.FormatInt(f.db.put("ws-open", "bob", dbclient.WorkspaceAccessPending), 10)

	require.Equal(t, http.StatusForbidden, f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "member", id, "",
		`{"comment":"ok"}`).Code)

	rsp := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "mgr", id, "", `{"comment":"welcome"}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	item := decodeItem(t, rsp)
	require.Equal(t, dbclient.WorkspaceAccessApproved, item.Status)
	require.Equal(t, "mgr", item.DecidedBy)
	require.Equal(t, "welcome", item.DecisionComment)

	user := &v1.User{}
	require.NoError(t, f.h.Get(context.Background(), client.ObjectKey{Name: "bob"}, user))
	require.True(t, commonuser.HasWorkspaceRight(user, "ws-open"))
	require.True(t, v1.IsContainRole(user.Spec.Roles, v1.DefaultRole))
	require.Equal(t, []string{"bob@example.com"}, f.db.recipients("approved"))

	again := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "admin", id, "", `{}`)
	require.Equal(t, http.StatusConflict, again.Code)
	require.Contains(t, again.Body.String(), "approved")
	require.Equal(t, http.StatusConflict, f.call(f.h.RejectWorkspaceAccessRequest, http.MethodPost, "admin", id, "", `{}`).Code)
}

// A requester never decides the own request, even as an approver of the workspace.
func TestSelfDecisionIsRefused(t *testing.T) {
	f := newAccessRequestFixture(t)
	id := strconv.FormatInt(f.db.put("ws-open", "mgr", dbclient.WorkspaceAccessPending), 10)
	for _, fn := range []gin.HandlerFunc{f.h.ApproveWorkspaceAccessRequest, f.h.RejectWorkspaceAccessRequest} {
		rsp := f.call(fn, http.MethodPost, "mgr", id, "", `{}`)
		require.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())
		require.Contains(t, rsp.Body.String(), "own")
	}
	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
}

// When the grant cannot be written the approval goes back to pending.
func TestApproveRevertsWhenTheGrantFails(t *testing.T) {
	f := newAccessRequestFixture(t)
	idNum := f.db.put("ws-open", "ghost", dbclient.WorkspaceAccessPending)
	rsp := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "mgr", strconv.FormatInt(idNum, 10), "", `{}`)
	require.Equal(t, http.StatusNotFound, rsp.Code, rsp.Body.String())
	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), idNum)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
	require.Empty(t, f.db.recipients("approved"))
}

// When the grant fails while the requester has another pending request for the workspace, the approved
// request is cancelled instead of pending, so at most one request stays pending.
func TestApproveRevertCancelsWhenAnotherRequestIsPending(t *testing.T) {
	f := newAccessRequestFixture(t)
	first := f.db.put("ws-open", "ghost", dbclient.WorkspaceAccessPending)
	second := f.db.put("ws-open", "ghost", dbclient.WorkspaceAccessPending)
	rsp := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "mgr", strconv.FormatInt(first, 10), "", `{}`)
	require.Equal(t, http.StatusNotFound, rsp.Code, rsp.Body.String())

	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), first)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessCancelled, req.Status)
	req, err = f.db.GetWorkspaceAccessRequest(context.Background(), second)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
}

// A revert that fails is retried until it succeeds.
func TestApproveRevertIsRetried(t *testing.T) {
	f := newAccessRequestFixture(t)
	f.db.revertFailures = 1
	idNum := f.db.put("ws-open", "ghost", dbclient.WorkspaceAccessPending)
	rsp := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "mgr", strconv.FormatInt(idNum, 10), "", `{}`)
	require.Equal(t, http.StatusNotFound, rsp.Code, rsp.Body.String())
	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), idNum)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
	require.Equal(t, 2, f.db.revertCalls)
}

// Once the workspace no longer accepts requests, a pending request may be rejected but not approved.
func TestDecideWhenThePolicyIsClosed(t *testing.T) {
	f := newAccessRequestFixture(t)
	approveId := strconv.FormatInt(f.db.put("ws-closed", "bob", dbclient.WorkspaceAccessPending), 10)
	rejectId := strconv.FormatInt(f.db.put("ws-closed", "alice", dbclient.WorkspaceAccessPending), 10)

	rsp := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "admin", approveId, "", `{}`)
	require.Equal(t, http.StatusConflict, rsp.Code, rsp.Body.String())
	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
	user := &v1.User{}
	require.NoError(t, f.h.Get(context.Background(), client.ObjectKey{Name: "bob"}, user))
	require.False(t, commonuser.HasWorkspaceRight(user, "ws-closed"))

	rsp = f.call(f.h.RejectWorkspaceAccessRequest, http.MethodPost, "admin", rejectId, "", `{}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
}

// A rejection closes the request and tells the requester.
func TestRejectWorkspaceAccessRequest(t *testing.T) {
	f := newAccessRequestFixture(t)
	id := strconv.FormatInt(f.db.put("ws-open", "bob", dbclient.WorkspaceAccessPending), 10)
	rsp := f.call(f.h.RejectWorkspaceAccessRequest, http.MethodPost, "admin", id, "", `{"comment":"no"}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, dbclient.WorkspaceAccessRejected, decodeItem(t, rsp).Status)
	require.Equal(t, []string{"bob@example.com"}, f.db.recipients("rejected"))
	user := &v1.User{}
	require.NoError(t, f.h.Get(context.Background(), client.ObjectKey{Name: "bob"}, user))
	require.False(t, commonuser.HasWorkspaceRight(user, "ws-open"))
}

// Only the requester cancels, and only a pending request.
func TestCancelWorkspaceAccessRequest(t *testing.T) {
	f := newAccessRequestFixture(t)
	id := strconv.FormatInt(f.db.put("ws-open", "alice", dbclient.WorkspaceAccessPending), 10)
	require.Equal(t, http.StatusForbidden, f.call(f.h.CancelWorkspaceAccessRequest, http.MethodPost, "mgr", id, "", "").Code)
	require.Equal(t, http.StatusForbidden, f.call(f.h.CancelWorkspaceAccessRequest, http.MethodPost, "bob", id, "", "").Code)

	rsp := f.call(f.h.CancelWorkspaceAccessRequest, http.MethodPost, "alice", id, "", "")
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.Equal(t, dbclient.WorkspaceAccessCancelled, decodeItem(t, rsp).Status)
	again := f.call(f.h.CancelWorkspaceAccessRequest, http.MethodPost, "alice", id, "", "")
	require.Equal(t, http.StatusConflict, again.Code)
	require.Contains(t, again.Body.String(), "cancelled")
}

// The mine view lists the caller's requests; the approver view lists the pending requests the caller decides.
func TestListWorkspaceAccessRequestViews(t *testing.T) {
	f := newAccessRequestFixture(t)
	f.db.put("ws-open", "alice", dbclient.WorkspaceAccessRejected)
	f.db.put("ws-open", "alice", dbclient.WorkspaceAccessPending)
	f.db.put("ws-other", "bob", dbclient.WorkspaceAccessPending)

	ids := func(userId, query string) []int64 {
		rsp := f.call(f.h.ListWorkspaceAccessRequests, http.MethodGet, userId, "", query, "")
		require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
		var got view.ListWorkspaceAccessRequestResponse
		require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
		require.Equal(t, len(got.Items), got.TotalCount)
		result := []int64{}
		for _, item := range got.Items {
			result = append(result, item.Id)
		}
		return result
	}
	require.Equal(t, []int64{2, 1}, ids("alice", "view=mine"))
	require.Equal(t, []int64{2, 1}, ids("alice", ""))
	require.Equal(t, []int64{2}, ids("mgr", "view=approver"))
	require.Equal(t, []int64{3, 2}, ids("admin", "view=approver"))
	require.Equal(t, []int64{}, ids("bob", "view=approver"))
	require.Equal(t, http.StatusBadRequest, f.call(f.h.ListWorkspaceAccessRequests, http.MethodGet, "alice", "", "view=all", "").Code)
}

// The catalog lists the approval workspaces with the caller's status, the managers and the teams.
func TestWorkspaceCatalog(t *testing.T) {
	f := newAccessRequestFixture(t)
	pendingId := f.db.put("ws-open", "alice", dbclient.WorkspaceAccessPending)

	catalog := func(userId string) map[string]view.WorkspaceCatalogItem {
		rsp := f.call(f.h.ListWorkspaceCatalog, http.MethodGet, userId, "", "", "")
		require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
		var got view.WorkspaceCatalogResponse
		require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
		require.Equal(t, len(got.Items), got.TotalCount)
		result := map[string]view.WorkspaceCatalogItem{}
		for _, item := range got.Items {
			result[item.WorkspaceId] = item
		}
		return result
	}
	alice := catalog("alice")
	require.Len(t, alice, 2, "the closed workspace is not in the catalog")
	require.Equal(t, view.CatalogStatusPending, alice["ws-open"].MyStatus)
	require.Equal(t, pendingId, *alice["ws-open"].PendingRequestId)
	require.Equal(t, view.CatalogStatusNone, alice["ws-other"].MyStatus)
	require.Nil(t, alice["ws-other"].PendingRequestId)
	require.Equal(t, []view.WorkspaceCatalogManager{{Id: "mgr", Name: "mgr", Email: "mgr@example.com"}},
		alice["ws-open"].Managers)
	require.Equal(t, []view.WorkspaceCatalogTeam{{Id: "t1", DisplayName: "t1", Role: v1.WorkspaceTeamMember}},
		alice["ws-open"].Teams)

	require.Equal(t, view.CatalogStatusMember, catalog("member")["ws-open"].MyStatus)
	require.Equal(t, view.CatalogStatusMember, catalog("mgr")["ws-open"].MyStatus)
	require.Equal(t, view.CatalogStatusMember, catalog("admin")["ws-other"].MyStatus)
	require.Equal(t, view.CatalogStatusNone, catalog("bob")["ws-open"].MyStatus)
}

// The catalog status follows the workspaces GET /users/self/workspaces reports: a default workspace the
// record holds is a membership, and a read-only administrator is a member only where the record says so.
func TestWorkspaceCatalogFollowsTheEffectiveWorkspaces(t *testing.T) {
	viper.Set("users.join_default_workspaces", true)
	def := genWorkspaceFixture("ws-default", "Default WS")
	def.Spec.AccessPolicy = v1.WorkspaceAccessApproval
	def.Spec.IsDefault = true
	f := newAccessRequestFixture(t, def,
		withEmail(genUserFixture("carol", []v1.UserRole{v1.DefaultRole}, []string{"ws-default"}, nil)),
		withEmail(genUserFixture("auditor", []v1.UserRole{v1.SystemAdminReadonlyRole}, []string{"ws-other"}, nil)))

	status := func(userId, workspace string) string {
		rsp := f.call(f.h.ListWorkspaceCatalog, http.MethodGet, userId, "", "", "")
		require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
		var got view.WorkspaceCatalogResponse
		require.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &got))
		for _, item := range got.Items {
			if item.WorkspaceId == workspace {
				return item.MyStatus
			}
		}
		return ""
	}
	require.Equal(t, view.CatalogStatusMember, status("carol", "ws-default"))
	require.Equal(t, view.CatalogStatusNone, status("alice", "ws-default"))
	require.Equal(t, view.CatalogStatusMember, status("auditor", "ws-other"))
	require.Equal(t, view.CatalogStatusNone, status("auditor", "ws-open"))
	require.Equal(t, view.CatalogStatusMember, status("admin", "ws-default"))
}

// A manager of the workspace through a team neither sees nor decides its requests: only the direct
// managers and the system administrators do.
func TestTeamManagerDoesNotDecideAccessRequests(t *testing.T) {
	f := newAccessRequestFixture(t, withEmail(genUserFixture("teammgr", []v1.UserRole{v1.DefaultRole}, nil, nil)))
	id := strconv.FormatInt(f.db.put("ws-open", "alice", dbclient.WorkspaceAccessPending), 10)
	overlay := &extauth.Overlay{UserId: "teammgr", Roles: []v1.UserRole{v1.DefaultRole},
		Managed: []string{"ws-open"}, ManagedSources: map[string][]string{"ws-open": {"team:t1"}}}
	asTeamManager := func(fn gin.HandlerFunc, method, query, body string) *httptest.ResponseRecorder {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(method, "/?"+query, strings.NewReader(body))
		c.Request = c.Request.WithContext(extauth.WithOverlay(c.Request.Context(), overlay))
		c.Set(common.UserId, "teammgr")
		c.Set(common.Name, id)
		fn(c)
		return rsp
	}

	require.Equal(t, http.StatusForbidden, asTeamManager(f.h.GetWorkspaceAccessRequest, http.MethodGet, "", "").Code)
	require.Equal(t, http.StatusForbidden, asTeamManager(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "", `{}`).Code)
	list := asTeamManager(f.h.ListWorkspaceAccessRequests, http.MethodGet, "view=approver", "")
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	var got view.ListWorkspaceAccessRequestResponse
	require.NoError(t, json.Unmarshal(list.Body.Bytes(), &got))
	require.Zero(t, got.TotalCount)
}

// frozenUser returns a user fixture whose record is frozen.
func frozenUser(id string, workspaces, managed []string) *v1.User {
	user := withEmail(genUserFixture(id, []v1.UserRole{v1.DefaultRole}, workspaces, managed))
	user.Spec.RestrictedType = v1.UserFrozen
	return user
}

// accessRoutes returns the seven access-request routes and the catalog, each run in ctx as the user on
// the request id.
func accessRoutes(f *accessRequestFixture, ctx context.Context, userId, id string) map[string]*httptest.ResponseRecorder {
	return map[string]*httptest.ResponseRecorder{
		"catalog": f.callIn(ctx, f.h.ListWorkspaceCatalog, http.MethodGet, userId, "", "", ""),
		"create": f.callIn(ctx, f.h.CreateWorkspaceAccessRequest, http.MethodPost, userId, "", "",
			`{"workspace":"ws-other"}`),
		"list":    f.callIn(ctx, f.h.ListWorkspaceAccessRequests, http.MethodGet, userId, "", "view=approver", ""),
		"get":     f.callIn(ctx, f.h.GetWorkspaceAccessRequest, http.MethodGet, userId, id, "", ""),
		"approve": f.callIn(ctx, f.h.ApproveWorkspaceAccessRequest, http.MethodPost, userId, id, "", `{}`),
		"reject":  f.callIn(ctx, f.h.RejectWorkspaceAccessRequest, http.MethodPost, userId, id, "", `{}`),
		"cancel":  f.callIn(ctx, f.h.CancelWorkspaceAccessRequest, http.MethodPost, userId, id, "", ""),
	}
}

// A user whose record is frozen is refused on every access-request route and the catalog: a frozen manager
// does not approve, and a frozen requester neither creates a request nor reads the catalog.
func TestFrozenUserIsRefusedOnAccessRequests(t *testing.T) {
	f := newAccessRequestFixture(t, frozenUser("fmgr", nil, []string{"ws-open"}), frozenUser("frozen", nil, nil))
	id := strconv.FormatInt(f.db.put("ws-open", "bob", dbclient.WorkspaceAccessPending), 10)

	for route, rsp := range accessRoutes(f, context.Background(), "fmgr", id) {
		require.Equal(t, http.StatusForbidden, rsp.Code, "%s: %s", route, rsp.Body.String())
		require.Contains(t, rsp.Body.String(), "restricted", route)
	}
	ownId := strconv.FormatInt(f.db.put("ws-open", "frozen", dbclient.WorkspaceAccessPending), 10)
	for route, rsp := range accessRoutes(f, context.Background(), "frozen", ownId) {
		require.Equal(t, http.StatusForbidden, rsp.Code, "%s: %s", route, rsp.Body.String())
	}

	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
	user := &v1.User{}
	require.NoError(t, f.h.Get(context.Background(), client.ObjectKey{Name: "bob"}, user))
	require.False(t, commonuser.HasWorkspaceRight(user, "ws-open"))
	reqs, err := f.db.ListWorkspaceAccessRequests(context.Background(), dbclient.WorkspaceAccessRequestFilter{UserID: "frozen"})
	require.NoError(t, err)
	require.Len(t, reqs, 1, "a frozen requester creates no request")
	require.Empty(t, f.db.notifications)
}

// A user the request's identity restricts is refused as a frozen record is: a manager restricted by the
// overlay does not approve, and a requester restricted by it neither creates a request nor reads the catalog.
func TestOverlayRestrictedUserIsRefusedOnAccessRequests(t *testing.T) {
	f := newAccessRequestFixture(t)
	id := strconv.FormatInt(f.db.put("ws-open", "bob", dbclient.WorkspaceAccessPending), 10)
	restricted := func(userId string) context.Context {
		return extauth.WithOverlay(context.Background(), &extauth.Overlay{UserId: userId, Restricted: true})
	}

	for route, rsp := range accessRoutes(f, restricted("mgr"), "mgr", id) {
		require.Equal(t, http.StatusForbidden, rsp.Code, "%s: %s", route, rsp.Body.String())
	}
	for route, rsp := range accessRoutes(f, restricted("alice"), "alice", id) {
		require.Equal(t, http.StatusForbidden, rsp.Code, "%s: %s", route, rsp.Body.String())
	}

	req, err := f.db.GetWorkspaceAccessRequest(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, dbclient.WorkspaceAccessPending, req.Status)
	user := &v1.User{}
	require.NoError(t, f.h.Get(context.Background(), client.ObjectKey{Name: "bob"}, user))
	require.False(t, commonuser.HasWorkspaceRight(user, "ws-open"))
	reqs, err := f.db.ListWorkspaceAccessRequests(context.Background(), dbclient.WorkspaceAccessRequestFilter{UserID: "alice"})
	require.NoError(t, err)
	require.Empty(t, reqs)

	rsp := f.callIn(context.Background(), f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "mgr", id, "", `{}`)
	require.Equal(t, http.StatusOK, rsp.Code, "without the restriction the manager approves: %s", rsp.Body.String())
}

// With users.base_roles [self-read], an approved requester holds the default role and the workspace: both are
// written to the stored record, so the builtin mode and a request with an overlay see them alike.
func TestApprovalGrantsTheDefaultRoleWithTheMinimalBaseRole(t *testing.T) {
	f := newAccessRequestFixture(t, withEmail(genUserFixture("newbie", []v1.UserRole{v1.SelfReadRole}, nil, nil)))
	viper.Set("users.base_roles", []string{string(v1.SelfReadRole)})
	id := strconv.FormatInt(f.db.put("ws-open", "newbie", dbclient.WorkspaceAccessPending), 10)

	rsp := f.call(f.h.ApproveWorkspaceAccessRequest, http.MethodPost, "mgr", id, "", `{}`)
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())

	record := &v1.User{}
	require.NoError(t, f.h.Get(context.Background(), client.ObjectKey{Name: "newbie"}, record))
	require.Equal(t, []v1.UserRole{v1.SelfReadRole, v1.DefaultRole}, record.Spec.Roles)
	require.True(t, commonuser.HasWorkspaceRight(record, "ws-open"))

	builtin, err := f.h.accessController.GetRequestUser(context.Background(), "newbie")
	require.NoError(t, err)
	require.True(t, v1.IsContainRole(builtin.Spec.Roles, v1.DefaultRole))
	require.True(t, commonuser.HasWorkspaceRight(builtin, "ws-open"))

	ctx := extauth.WithOverlay(context.Background(), extauth.MapGroups("newbie", nil, nil, false))
	external, err := f.h.accessController.GetRequestUser(ctx, "newbie")
	require.NoError(t, err)
	require.Equal(t, []v1.UserRole{v1.SelfReadRole, v1.DefaultRole}, external.Spec.Roles)
	require.True(t, commonuser.HasWorkspaceRight(external, "ws-open"))
}
