/*
 * Copyright (C) 2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/topic/workspaceaccess"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	// errAccessRequestHidden answers a request id that does not exist and one the caller may not see alike.
	errAccessRequestHidden = "the access request does not exist or you are not its requester or approver"
	// errAccessPolicyRefused answers a workspace that does not accept requests and a missing one alike.
	errAccessPolicyRefused = "the workspace does not accept access requests"
	errCrossSiteRequest    = "the request must be sent from the console"

	maxAccessReasonLength  = 2000
	maxAccessCommentLength = 2000

	// accessRevertTimeout bounds the revert of an approval whose grant failed, which may run after
	// the request context ended.
	accessRevertTimeout = 30 * time.Second
	// accessRevertMaxElapsed and accessRevertMaxInterval bound the backoff of a failed revert.
	accessRevertMaxElapsed  = 20 * time.Second
	accessRevertMaxInterval = 5 * time.Second
)

// ListWorkspaceCatalog lists the workspaces that accept access requests, with the caller's status in each.
func (h *Handler) ListWorkspaceCatalog(c *gin.Context) {
	handle(c, h.listWorkspaceCatalog)
}

// CreateWorkspaceAccessRequest submits a request of the caller for member access to a workspace.
func (h *Handler) CreateWorkspaceAccessRequest(c *gin.Context) {
	handle(c, h.createWorkspaceAccessRequest)
}

// ListWorkspaceAccessRequests lists the caller's requests, or the pending requests the caller may decide.
func (h *Handler) ListWorkspaceAccessRequests(c *gin.Context) {
	handle(c, h.listWorkspaceAccessRequests)
}

// GetWorkspaceAccessRequest returns a request to its requester or to an approver.
func (h *Handler) GetWorkspaceAccessRequest(c *gin.Context) {
	handle(c, h.getWorkspaceAccessRequest)
}

// ApproveWorkspaceAccessRequest approves a pending request and grants the requester the workspace.
func (h *Handler) ApproveWorkspaceAccessRequest(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		return h.decideWorkspaceAccessRequest(c, dbclient.WorkspaceAccessApproved)
	})
}

// RejectWorkspaceAccessRequest rejects a pending request.
func (h *Handler) RejectWorkspaceAccessRequest(c *gin.Context) {
	handle(c, func(c *gin.Context) (interface{}, error) {
		return h.decideWorkspaceAccessRequest(c, dbclient.WorkspaceAccessRejected)
	})
}

// CancelWorkspaceAccessRequest withdraws a pending request of the caller.
func (h *Handler) CancelWorkspaceAccessRequest(c *gin.Context) {
	handle(c, h.cancelWorkspaceAccessRequest)
}

// listWorkspaceCatalog implements GET /workspaces/catalog.
func (h *Handler) listWorkspaceCatalog(c *gin.Context) (interface{}, error) {
	requestUser, err := h.accessRequestUser(c)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	workspaceList := &v1.WorkspaceList{}
	if err = h.List(ctx, workspaceList); err != nil {
		return nil, err
	}
	var catalog []*v1.Workspace
	for i := range workspaceList.Items {
		if workspaceList.Items[i].Spec.AccessPolicy == v1.WorkspaceAccessApproval {
			catalog = append(catalog, &workspaceList.Items[i])
		}
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Name < catalog[j].Name })
	result := &view.WorkspaceCatalogResponse{Items: []view.WorkspaceCatalogItem{}}
	if len(catalog) == 0 {
		return result, nil
	}
	userList := &v1.UserList{}
	if err = h.List(ctx, userList); err != nil {
		return nil, err
	}
	pending := map[string]int64{}
	if h.dbClient != nil {
		reqs, listErr := h.dbClient.ListWorkspaceAccessRequests(ctx, dbclient.WorkspaceAccessRequestFilter{
			UserID: requestUser.Name, Status: dbclient.WorkspaceAccessPending})
		if listErr != nil {
			return nil, listErr
		}
		for _, req := range reqs {
			if _, ok := pending[req.Workspace]; !ok {
				pending[req.Workspace] = req.ID
			}
		}
	}
	held, err := h.effectiveWorkspaces(ctx, requestUser)
	if err != nil {
		return nil, err
	}
	var teamIds []string
	for _, ws := range catalog {
		teamIds = append(teamIds, teamNames(ws)...)
	}
	display := h.teamDisplay(ctx, teamIds)
	for _, ws := range catalog {
		item := view.WorkspaceCatalogItem{
			WorkspaceId:   ws.Name,
			WorkspaceName: workspaceLabel(ws),
			Description:   v1.GetDescription(ws),
			Managers:      directWorkspaceManagers(ws, userList.Items),
			Teams:         []view.WorkspaceCatalogTeam{},
			MyStatus:      view.CatalogStatusNone,
		}
		for _, id := range teamNames(ws) {
			role, _ := ws.TeamRole(id)
			team := view.WorkspaceCatalogTeam{Id: id, DisplayName: id, Role: role}
			if info, ok := display[id]; ok && info.DisplayName != "" {
				team.DisplayName = info.DisplayName
			}
			item.Teams = append(item.Teams, team)
		}
		if requestUser.IsSystemAdmin() || held[ws.Name] || slices.Contains(ws.Spec.Managers, requestUser.Name) {
			item.MyStatus = view.CatalogStatusMember
		} else if id, ok := pending[ws.Name]; ok {
			item.MyStatus = view.CatalogStatusPending
			item.PendingRequestId = &id
		}
		result.Items = append(result.Items, item)
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// createWorkspaceAccessRequest implements POST /workspace-access-requests.
func (h *Handler) createWorkspaceAccessRequest(c *gin.Context) (interface{}, error) {
	if err := requireSameOrigin(c); err != nil {
		return nil, err
	}
	requestUser, err := h.accessRequestUser(c)
	if err != nil {
		return nil, err
	}
	req := &view.CreateWorkspaceAccessRequest{}
	if _, err = apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, err
	}
	req.Workspace = strings.TrimSpace(req.Workspace)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Workspace == "" {
		return nil, commonerrors.NewBadRequest("the workspace is empty")
	}
	if utf8.RuneCountInString(req.Reason) > maxAccessReasonLength {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("the reason is longer than %d characters", maxAccessReasonLength))
	}
	ctx := c.Request.Context()
	workspace, err := h.findWorkspace(ctx, req.Workspace)
	if err != nil {
		return nil, err
	}
	if workspace == nil || workspace.Spec.AccessPolicy != v1.WorkspaceAccessApproval {
		return nil, commonerrors.NewForbidden(errAccessPolicyRefused)
	}
	if hasWorkspaceAccess(requestUser, workspace) {
		return nil, commonerrors.NewConflict("you already have access to the workspace " + workspace.Name)
	}
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}
	record := &dbclient.WorkspaceAccessRequest{
		Workspace: workspace.Name,
		UserID:    requestUser.Name,
		UserName:  v1.GetUserName(requestUser),
		Reason:    req.Reason,
		Status:    dbclient.WorkspaceAccessPending,
		CreatedAt: time.Now().UTC(),
	}
	if err = h.dbClient.CreateWorkspaceAccessRequest(ctx, record); err != nil {
		var pending *dbclient.PendingAccessRequestError
		if errors.As(err, &pending) {
			return nil, commonerrors.NewConflict(fmt.Sprintf(
				"you already have a pending access request %d for the workspace %s", pending.ID, workspace.Name))
		}
		return nil, err
	}
	klog.Infof("create workspace access request, id: %d, workspace: %s, user: %s", record.ID, record.Workspace, record.UserID)
	item := h.accessRequestItems(ctx, []*dbclient.WorkspaceAccessRequest{record})[0]
	h.notifyAccessRequest(ctx, workspaceaccess.EventCreated, &item, h.approverEmails(ctx, workspace))
	c.Status(http.StatusCreated)
	return &item, nil
}

// listWorkspaceAccessRequests implements GET /workspace-access-requests?view=mine|approver.
func (h *Handler) listWorkspaceAccessRequests(c *gin.Context) (interface{}, error) {
	requestUser, err := h.accessRequestUser(c)
	if err != nil {
		return nil, err
	}
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("database is not enabled")
	}
	ctx := c.Request.Context()
	var reqs []*dbclient.WorkspaceAccessRequest
	switch c.Query("view") {
	case "", view.AccessRequestViewMine:
		reqs, err = h.dbClient.ListWorkspaceAccessRequests(ctx, dbclient.WorkspaceAccessRequestFilter{UserID: requestUser.Name})
	case view.AccessRequestViewApprover:
		reqs, err = h.approverPendingRequests(ctx, requestUser)
	default:
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid view %q, supported: [%s, %s]",
			c.Query("view"), view.AccessRequestViewMine, view.AccessRequestViewApprover))
	}
	if err != nil {
		return nil, err
	}
	items := h.accessRequestItems(ctx, reqs)
	return &view.ListWorkspaceAccessRequestResponse{TotalCount: len(items), Items: items}, nil
}

// approverPendingRequests returns the pending requests of the workspaces the user may decide, except the
// user's own: every workspace for a system administrator, the directly managed ones otherwise.
func (h *Handler) approverPendingRequests(ctx context.Context, user *v1.User) ([]*dbclient.WorkspaceAccessRequest, error) {
	filter := dbclient.WorkspaceAccessRequestFilter{Status: dbclient.WorkspaceAccessPending}
	if !user.IsSystemAdmin() {
		record, err := h.getAdminUser(ctx, user.Name)
		if err != nil {
			return nil, err
		}
		managed := slices.Clone(commonuser.GetManagedWorkspace(record))
		workspaceList := &v1.WorkspaceList{}
		if err := h.List(ctx, workspaceList); err != nil {
			return nil, err
		}
		for i := range workspaceList.Items {
			if slices.Contains(workspaceList.Items[i].Spec.Managers, user.Name) {
				managed = append(managed, workspaceList.Items[i].Name)
			}
		}
		sort.Strings(managed)
		filter.Workspaces = slices.Compact(managed)
		if filter.Workspaces == nil {
			filter.Workspaces = []string{}
		}
	}
	reqs, err := h.dbClient.ListWorkspaceAccessRequests(ctx, filter)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(reqs, func(r *dbclient.WorkspaceAccessRequest) bool { return r.UserID == user.Name }), nil
}

// getWorkspaceAccessRequest implements GET /workspace-access-requests/:id.
func (h *Handler) getWorkspaceAccessRequest(c *gin.Context) (interface{}, error) {
	requestUser, err := h.accessRequestUser(c)
	if err != nil {
		return nil, err
	}
	req, _, isRequester, isApprover, err := h.loadVisibleAccessRequest(c, requestUser)
	if err != nil {
		return nil, err
	}
	return &view.WorkspaceAccessRequestDetail{
		WorkspaceAccessRequestItem: h.accessRequestItems(c.Request.Context(), []*dbclient.WorkspaceAccessRequest{req})[0],
		CanDecide:                  isApprover && !isRequester && req.Status == dbclient.WorkspaceAccessPending,
		IsRequester:                isRequester,
	}, nil
}

// decideWorkspaceAccessRequest implements the approve and reject requests; to is the new status.
// The status changes first, in a conditional update, so only one decision takes effect. An approval
// then grants the workspace, and is reverted by revertApproval when the grant cannot be written. A workspace
// that no longer accepts requests may still have them rejected, not approved.
func (h *Handler) decideWorkspaceAccessRequest(c *gin.Context, to string) (interface{}, error) {
	if err := requireSameOrigin(c); err != nil {
		return nil, err
	}
	requestUser, err := h.accessRequestUser(c)
	if err != nil {
		return nil, err
	}
	body := &view.DecideWorkspaceAccessRequest{}
	if _, err = apiutils.ParseRequestBody(c.Request, body); err != nil {
		return nil, err
	}
	body.Comment = strings.TrimSpace(body.Comment)
	if utf8.RuneCountInString(body.Comment) > maxAccessCommentLength {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("the comment is longer than %d characters", maxAccessCommentLength))
	}
	req, workspace, isRequester, _, err := h.loadVisibleAccessRequest(c, requestUser)
	if err != nil {
		return nil, err
	}
	if isRequester {
		return nil, commonerrors.NewForbidden("you may not decide your own access request")
	}
	if req.Status != dbclient.WorkspaceAccessPending {
		return nil, accessRequestNotPending(req.Status)
	}
	if to == dbclient.WorkspaceAccessApproved && workspace == nil {
		return nil, commonerrors.NewConflict("the workspace " + req.Workspace + " no longer exists")
	}
	if to == dbclient.WorkspaceAccessApproved && workspace.Spec.AccessPolicy != v1.WorkspaceAccessApproval {
		return nil, commonerrors.NewConflict("the workspace " + req.Workspace + " no longer accepts access requests")
	}
	ctx := c.Request.Context()
	updated, err := h.transitionAccessRequest(ctx, req.ID, dbclient.WorkspaceAccessTransition{
		From: dbclient.WorkspaceAccessPending, To: to, DecidedBy: requestUser.Name, Comment: body.Comment})
	if err != nil {
		return nil, err
	}
	if to == dbclient.WorkspaceAccessApproved {
		if err = h.grantWorkspaceAccess(ctx, updated.UserID, updated.Workspace); err != nil {
			klog.ErrorS(err, "failed to grant the approved workspace access", "id", updated.ID,
				"user", updated.UserID, "workspace", updated.Workspace)
			h.revertApproval(ctx, updated.ID)
			return nil, err
		}
	}
	klog.Infof("decide workspace access request, id: %d, status: %s, decided by: %s", updated.ID, to, requestUser.Name)
	item := h.accessRequestItems(ctx, []*dbclient.WorkspaceAccessRequest{updated})[0]
	event := workspaceaccess.EventRejected
	if to == dbclient.WorkspaceAccessApproved {
		event = workspaceaccess.EventApproved
	}
	h.notifyAccessRequest(ctx, event, &item, nonEmpty(item.UserEmail))
	return &item, nil
}

// cancelWorkspaceAccessRequest implements POST /workspace-access-requests/:id/cancel.
func (h *Handler) cancelWorkspaceAccessRequest(c *gin.Context) (interface{}, error) {
	if err := requireSameOrigin(c); err != nil {
		return nil, err
	}
	requestUser, err := h.accessRequestUser(c)
	if err != nil {
		return nil, err
	}
	req, _, isRequester, _, err := h.loadVisibleAccessRequest(c, requestUser)
	if err != nil {
		return nil, err
	}
	if !isRequester {
		return nil, commonerrors.NewForbidden("only the requester may cancel the access request")
	}
	if req.Status != dbclient.WorkspaceAccessPending {
		return nil, accessRequestNotPending(req.Status)
	}
	ctx := c.Request.Context()
	updated, err := h.transitionAccessRequest(ctx, req.ID, dbclient.WorkspaceAccessTransition{
		From: dbclient.WorkspaceAccessPending, To: dbclient.WorkspaceAccessCancelled, DecidedBy: requestUser.Name})
	if err != nil {
		return nil, err
	}
	klog.Infof("cancel workspace access request, id: %d, user: %s", updated.ID, requestUser.Name)
	item := h.accessRequestItems(ctx, []*dbclient.WorkspaceAccessRequest{updated})[0]
	return &item, nil
}

// accessRequestUser returns the request user with the overlay merged in; an unknown user is unauthorized
// and a restricted one, by the record or by the overlay, is forbidden.
func (h *Handler) accessRequestUser(c *gin.Context) (*v1.User, error) {
	user, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, commonerrors.NewUnauthorized("the request user is unknown")
	}
	if user.IsRestricted() {
		return nil, commonerrors.NewForbidden(fmt.Sprintf("The user is restricted. type: %d", user.Spec.RestrictedType))
	}
	return user, nil
}

// loadVisibleAccessRequest reads the request named by the route and the workspace it is for, which is nil
// when the workspace no longer exists. A missing request and one the user is neither the requester nor an
// approver of get the same 403.
func (h *Handler) loadVisibleAccessRequest(c *gin.Context, user *v1.User) (
	req *dbclient.WorkspaceAccessRequest, workspace *v1.Workspace, isRequester, isApprover bool, err error) {
	hidden := commonerrors.NewForbidden(errAccessRequestHidden)
	id, parseErr := strconv.ParseInt(c.GetString(common.Name), 10, 64)
	if parseErr != nil || id <= 0 {
		return nil, nil, false, false, hidden
	}
	if h.dbClient == nil {
		return nil, nil, false, false, commonerrors.NewInternalError("database is not enabled")
	}
	ctx := c.Request.Context()
	if req, err = h.dbClient.GetWorkspaceAccessRequest(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, false, hidden
		}
		return nil, nil, false, false, err
	}
	if workspace, err = h.findWorkspace(ctx, req.Workspace); err != nil {
		return nil, nil, false, false, err
	}
	record, err := h.getAdminUser(ctx, user.Name)
	if err != nil {
		return nil, nil, false, false, err
	}
	isRequester = req.UserID == user.Name
	isApprover = isAccessApprover(user, record, req.Workspace, workspace)
	if !isRequester && !isApprover {
		return nil, nil, false, false, hidden
	}
	return req, workspace, isRequester, isApprover, nil
}

// transitionAccessRequest applies the transition and answers 409 with the current status when the request
// has left the From state meanwhile.
func (h *Handler) transitionAccessRequest(ctx context.Context, id int64,
	t dbclient.WorkspaceAccessTransition) (*dbclient.WorkspaceAccessRequest, error) {
	updated, err := h.dbClient.TransitionWorkspaceAccessRequest(ctx, id, t)
	if err == nil {
		return updated, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	current, getErr := h.dbClient.GetWorkspaceAccessRequest(ctx, id)
	if getErr != nil {
		return nil, commonerrors.NewConflict("the access request is no longer " + t.From)
	}
	return nil, accessRequestNotPending(current.Status)
}

// revertApproval moves an approved request back to pending after its grant failed, so it can be decided again,
// or to cancelled when the requester has another pending request for the workspace. A failed revert is
// retried with backoff.
func (h *Handler) revertApproval(ctx context.Context, id int64) {
	revertCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), accessRevertTimeout)
	defer cancel()
	var err error
	_ = backoff.Retry(func() error {
		_, err = h.dbClient.RevertWorkspaceAccessApproval(revertCtx, id)
		if err == nil || errors.Is(err, gorm.ErrRecordNotFound) || revertCtx.Err() != nil {
			return nil
		}
		return err
	}, accessRevertMaxElapsed, accessRevertMaxInterval)
	if err != nil {
		klog.ErrorS(err, "failed to revert the approval of a workspace access request whose grant failed", "id", id)
	}
}

// grantWorkspaceAccess adds the workspace and the default role to the stored user record, reading the
// record again on a write conflict. A record that stores the system-admin role holds that role alone, so
// it is given the workspace only.
func (h *Handler) grantWorkspaceAccess(ctx context.Context, userId, workspace string) error {
	return backoff.ConflictRetry(func() error {
		user := &v1.User{}
		if err := h.Get(ctx, client.ObjectKey{Name: userId}, user); err != nil {
			if apierrors.IsNotFound(err) {
				return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("the requester %s has no user record", userId))
			}
			return err
		}
		changed := commonuser.AddWorkspace(user, workspace)
		if !user.IsSystemAdmin() && !v1.IsContainRole(user.Spec.Roles, v1.DefaultRole) {
			user.Spec.Roles = append(user.Spec.Roles, v1.DefaultRole)
			changed = true
		}
		if !changed {
			return nil
		}
		return h.Update(ctx, user)
	}, defaultRetryCount, defaultRetryDelay)
}

// findWorkspace returns the workspace, or nil when it does not exist.
func (h *Handler) findWorkspace(ctx context.Context, name string) (*v1.Workspace, error) {
	workspace := &v1.Workspace{}
	if err := h.Get(ctx, client.ObjectKey{Name: name}, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return workspace, nil
}

// accessRequestItems converts the requests to the API items, with the workspace names, the requester
// e-mails and the names of the deciders read from the cluster.
func (h *Handler) accessRequestItems(ctx context.Context, reqs []*dbclient.WorkspaceAccessRequest) []view.WorkspaceAccessRequestItem {
	workspaceNames := map[string]string{}
	users := map[string]*v1.User{}
	workspaceName := func(id string) string {
		if name, ok := workspaceNames[id]; ok {
			return name
		}
		name := id
		if ws, err := h.findWorkspace(ctx, id); err == nil && ws != nil {
			name = workspaceLabel(ws)
		}
		workspaceNames[id] = name
		return name
	}
	userOf := func(id string) *v1.User {
		if user, ok := users[id]; ok {
			return user
		}
		user := &v1.User{}
		if err := h.Get(ctx, client.ObjectKey{Name: id}, user); err != nil {
			user = nil
		}
		users[id] = user
		return user
	}
	items := make([]view.WorkspaceAccessRequestItem, 0, len(reqs))
	for _, req := range reqs {
		item := view.WorkspaceAccessRequestItem{
			Id:              req.ID,
			Workspace:       req.Workspace,
			WorkspaceName:   workspaceName(req.Workspace),
			UserId:          req.UserID,
			UserName:        req.UserName,
			Reason:          req.Reason,
			Status:          req.Status,
			DecidedBy:       req.DecidedBy,
			DecisionComment: req.DecisionComment,
			DecidedAt:       timeutil.FormatRFC3339(req.DecidedAt.Time),
			CreationTime:    timeutil.FormatRFC3339(req.CreatedAt),
			UpdateTime:      timeutil.FormatRFC3339(req.UpdatedAt.Time),
			ApprovalUrl:     workspaceaccess.RequestURL(req.ID),
		}
		if user := userOf(req.UserID); user != nil {
			item.UserEmail = v1.GetUserEmail(user)
			if item.UserName == "" {
				item.UserName = v1.GetUserName(user)
			}
		}
		if item.UserName == "" {
			item.UserName = req.UserID
		}
		if req.DecidedBy != "" {
			item.DecidedByName = req.DecidedBy
			if user := userOf(req.DecidedBy); user != nil && v1.GetUserName(user) != "" {
				item.DecidedByName = v1.GetUserName(user)
			}
		}
		items = append(items, item)
	}
	return items
}

// approverEmails returns the e-mail addresses the new request of the workspace is sent to: the direct
// managers (Spec.Managers and the users whose managed workspaces name it). Without any, it returns those
// of the system administrators.
func (h *Handler) approverEmails(ctx context.Context, workspace *v1.Workspace) []string {
	userList := &v1.UserList{}
	if err := h.List(ctx, userList); err != nil {
		klog.ErrorS(err, "failed to list the users to notify of a workspace access request")
		return nil
	}
	var emails []string
	for _, m := range directWorkspaceManagers(workspace, userList.Items) {
		emails = append(emails, m.Email)
	}
	if emails = nonEmpty(emails...); len(emails) > 0 {
		return emails
	}
	for i := range userList.Items {
		user := &userList.Items[i]
		if user.IsSystemAdmin() || v1.IsContainRole(h.recordedOverlay(ctx, user).Roles, v1.SystemAdminRole) {
			emails = append(emails, v1.GetUserEmail(user))
		}
	}
	return nonEmpty(emails...)
}

// notifyAccessRequest submits the notification of the event to the recipients. Failures are only logged.
func (h *Handler) notifyAccessRequest(ctx context.Context, event string, item *view.WorkspaceAccessRequestItem, to []string) {
	if h.dbClient == nil || !commonconfig.IsNotificationEnable() || len(to) == 0 {
		return
	}
	data := &workspaceaccess.TopicData{
		Event:         event,
		RequestId:     item.Id,
		Workspace:     item.Workspace,
		WorkspaceName: item.WorkspaceName,
		UserId:        item.UserId,
		UserName:      item.UserName,
		UserEmail:     item.UserEmail,
		Reason:        item.Reason,
		DecidedByName: item.DecidedByName,
		Comment:       item.DecisionComment,
		URL:           item.ApprovalUrl,
		To:            to,
	}
	if err := workspaceaccess.Submit(ctx, h.dbClient, data); err != nil {
		klog.ErrorS(err, "failed to submit the workspace access request notification", "id", item.Id, "event", event)
	}
}

// directWorkspaceManagers returns the users that manage the workspace directly: the ones in Spec.Managers
// and the ones whose managed workspaces name it, sorted by id.
func directWorkspaceManagers(workspace *v1.Workspace, users []v1.User) []view.WorkspaceCatalogManager {
	result := []view.WorkspaceCatalogManager{}
	for i := range users {
		user := &users[i]
		if !slices.Contains(workspace.Spec.Managers, user.Name) && !commonuser.HasWorkspaceManagedRight(user, workspace.Name) {
			continue
		}
		result = append(result, view.WorkspaceCatalogManager{
			Id: user.Name, Name: v1.GetUserName(user), Email: v1.GetUserEmail(user),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result
}

// effectiveWorkspaces returns the ids of the workspaces the user accesses or manages, as
// GET /users/self/workspaces reports them. user is the request user with the overlay merged in.
func (h *Handler) effectiveWorkspaces(ctx context.Context, user *v1.User) (map[string]bool, error) {
	record, err := h.getAdminUser(ctx, user.Name)
	if err != nil {
		return nil, err
	}
	workspaces, managed := h.userWorkspacesWithSources(ctx, user, record, extauth.RequestOverlay(ctx, user))
	held := map[string]bool{}
	for _, entry := range append(workspaces, managed...) {
		held[entry.Id] = true
	}
	return held, nil
}

// hasWorkspaceAccess reports whether the user, with the overlay merged in, already holds the workspace:
// as a system administrator, a member or a manager, directly or through a team or a group.
func hasWorkspaceAccess(user *v1.User, workspace *v1.Workspace) bool {
	return user.IsSystemAdmin() ||
		commonuser.HasWorkspaceRight(user, workspace.Name) ||
		commonuser.HasWorkspaceManagedRight(user, workspace.Name) ||
		slices.Contains(workspace.Spec.Managers, user.Name)
}

// isAccessApprover reports whether the user may decide the requests of the workspace: a system
// administrator (user, with the overlay merged in) or a direct manager of it (record, the stored user,
// or Spec.Managers). Management through a team or a group does not count. workspace is nil when it no
// longer exists.
func isAccessApprover(user, record *v1.User, workspaceId string, workspace *v1.Workspace) bool {
	if user.IsSystemAdmin() || commonuser.HasWorkspaceManagedRight(record, workspaceId) {
		return true
	}
	return workspace != nil && slices.Contains(workspace.Spec.Managers, user.Name)
}

// accessRequestNotPending returns the 409 for a request that is no longer pending.
func accessRequestNotPending(status string) error {
	return commonerrors.NewConflict("the access request is " + status + ", not pending")
}

// workspaceLabel returns the display name of the workspace, or its id.
func workspaceLabel(workspace *v1.Workspace) string {
	if name := v1.GetDisplayName(workspace); name != "" {
		return name
	}
	return workspace.Name
}

// nonEmpty returns the distinct non-empty values, sorted.
func nonEmpty(values ...string) []string {
	var result []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return slices.Compact(result)
}

// requireSameOrigin refuses a request a browser sent from another site: one whose Origin (scheme and host)
// is neither the console origin nor the request's own origin, an opaque Origin ("null"), or, without
// Origin, one marked cross-site by Sec-Fetch-Site. The request's own origin is its Host with the scheme of
// the connection, or of X-Forwarded-Proto when that is http or https. Clients that send neither header,
// such as API clients, are not browsers and pass.
func requireSameOrigin(c *gin.Context) error {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		if strings.EqualFold(c.GetHeader("Sec-Fetch-Site"), "cross-site") {
			return commonerrors.NewForbidden(errCrossSiteRequest)
		}
		return nil
	}
	got, ok := normalizeOrigin(origin)
	if !ok {
		return commonerrors.NewForbidden(errCrossSiteRequest)
	}
	scheme := "http"
	proto := strings.ToLower(strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]))
	if c.Request.TLS != nil {
		scheme = "https"
	} else if proto == "http" || proto == "https" {
		scheme = proto
	}
	allowed := []string{scheme + "://" + c.Request.Host, commonconfig.GetConsoleOrigin()}
	for _, candidate := range allowed {
		if want, valid := normalizeOrigin(candidate); valid && want == got {
			return nil
		}
	}
	return commonerrors.NewForbidden(errCrossSiteRequest)
}

// normalizeOrigin returns the origin as lower-case scheme://host, and false unless it is an http or https
// URL with a host.
func normalizeOrigin(origin string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), true
}
