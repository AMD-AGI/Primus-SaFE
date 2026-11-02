/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// accessFixture is a cluster with the built-in roles as the chart ships them (not a copy), a cluster, an
// addon, an addon template, and one user of each kind. The handlers are called directly, without the gate
// every authenticated route starts with (that is swept in the handlers package): what is checked here is
// the handlers' own checks, which are what a user who holds access is held to.
type accessFixture struct {
	h   *Handler
	cli client.Client
}

const (
	userSelfRead = "self-read-user"
	userDefault  = "default-user"
	userAdmin    = "admin-user"
	userOther    = "other-user"
	// userReadonly is a read-only administrator in a deployment whose base role is self-read: it holds both.
	userReadonly = "readonly-user"
)

func newAccessFixture(t *testing.T) *accessFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	user := func(name string, roles ...v1.UserRole) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}, Annotations: map[string]string{v1.UserNameAnnotation: name}},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
		}
	}
	objs := append(authtest.ChartRoles(t),
		user(userSelfRead, v1.SelfReadRole), user(userDefault, v1.DefaultRole), user(userAdmin, v1.SystemAdminRole), user(userOther, v1.DefaultRole),
		user(userReadonly, v1.SystemAdminReadonlyRole, v1.SelfReadRole),
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}},
		&v1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "a1"}},
		&v1.AddonTemplate{ObjectMeta: metav1.ObjectMeta{Name: "t1"}},
	)
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &accessFixture{
		h:   &Handler{Client: cli, accessController: &authority.AccessController{Client: cli}},
		cli: cli,
	}
}

// ctx builds the request context of a handler call: the user the credential named, the name in the path,
// and, when the route has one, the addon name.
func (f *accessFixture) ctx(userId, name, body string) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	c.Set(common.Name, name)
	c.Params = gin.Params{{Key: common.AddonName, Value: "a1"}}
	return c
}

func (f *accessFixture) notification(t *testing.T, userId string) bool {
	t.Helper()
	u := &v1.User{}
	require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: userId}, u))
	return v1.IsUserEnableNotification(u)
}

// The reads of clusters, addons and addon templates were answered to every authenticated user: the handlers
// did not check a role. They check now, so what a user may read is what the roles grant: the default role
// grants it (as it always effectively did, so existing users see no change), the self-read role does not.
func TestClusterAddonAndTemplateReadsAreChecked(t *testing.T) {
	f := newAccessFixture(t)
	for _, tt := range []struct {
		name string
		call func(c *gin.Context) (any, error)
	}{
		{"list clusters", f.h.listCluster},
		{"get a cluster", f.h.getCluster},
		{"list addons", f.h.listAddon},
		{"get an addon", f.h.getAddon},
		{"list addon templates", f.h.listAddonTemplate},
		{"get an addon template", f.h.getAddonTemplate},
	} {
		names := map[string]string{"get a cluster": "c1", "get an addon template": "t1"}
		for _, who := range []struct {
			user string
			ok   bool
		}{{userSelfRead, false}, {userDefault, true}, {userAdmin, true}, {"nobody-knows-me", false}} {
			t.Run(tt.name+" as "+who.user, func(t *testing.T) {
				res, err := tt.call(f.ctx(who.user, names[tt.name], ""))
				if who.ok {
					require.NoError(t, err)
					require.NotNil(t, res)
					return
				}
				require.Error(t, err)
				if who.user == userSelfRead {
					require.True(t, apierrors.IsForbidden(err), "refused with 403, not by chance: %v", err)
				}
				require.Nil(t, res, "nothing is read for a user who is refused")
			})
		}
	}
}

// The notification settings are the user's own: reading them takes the right to read the user's own record,
// changing them the right to update it. The write is judged by the record, not by the answer: a refused
// user's settings are left as they were.
func TestUserSettingsAreChecked(t *testing.T) {
	f := newAccessFixture(t)
	on := `{"enableNotification": true}`

	t.Run("a user who may update the own record reads and changes the own settings", func(t *testing.T) {
		res, err := f.h.getUserSettings(f.ctx(userDefault, common.UserSelf, ""))
		require.NoError(t, err)
		require.False(t, res.(*view.UserSettingsResponse).EnableNotification)
		_, err = f.h.updateUserSettings(f.ctx(userDefault, common.UserSelf, on))
		require.NoError(t, err)
		require.True(t, f.notification(t, userDefault))
	})

	// A read-only administrator holds a role that reads everything and neither update: the own settings are
	// read, as the other users' are, and cannot be changed. A reader of the own record gets the same from the
	// self-read role next to it: what keeps a user who holds that role alone off this route is the gate every
	// authenticated route starts with (swept in the handlers package), not the handler.
	for _, who := range []string{userReadonly, userSelfRead} {
		t.Run(who+" reads the own settings and cannot change them", func(t *testing.T) {
			res, err := f.h.getUserSettings(f.ctx(who, common.UserSelf, ""))
			require.NoError(t, err)
			require.False(t, res.(*view.UserSettingsResponse).EnableNotification)
			_, err = f.h.updateUserSettings(f.ctx(who, common.UserSelf, on))
			require.True(t, apierrors.IsForbidden(err), "%v", err)
			require.False(t, f.notification(t, who), "the record was written for a user who was refused")
		})
	}

	t.Run("a user nobody knows reads nothing", func(t *testing.T) {
		res, err := f.h.getUserSettings(f.ctx("nobody-knows-me", common.UserSelf, ""))
		require.Error(t, err)
		require.Nil(t, res)
	})

	t.Run("the check comes before the body is read", func(t *testing.T) {
		// A body that is not even JSON: a user who is refused is told that, not that the body is bad.
		_, err := f.h.updateUserSettings(f.ctx(userSelfRead, common.UserSelf, "not json"))
		require.True(t, apierrors.IsForbidden(err), "%v", err)
	})

	t.Run("another user's settings are for those who may update that user's identity", func(t *testing.T) {
		// A user who holds the default role could read anyone's flag before: it is system administrators now.
		for _, who := range []string{userSelfRead, userDefault} {
			res, err := f.h.getUserSettings(f.ctx(who, userOther, ""))
			require.True(t, apierrors.IsForbidden(err), "%s: %v", who, err)
			require.Nil(t, res)
			_, err = f.h.updateUserSettings(f.ctx(who, userOther, on))
			require.True(t, apierrors.IsForbidden(err), "%s: %v", who, err)
		}
		require.False(t, f.notification(t, userOther))

		_, err := f.h.getUserSettings(f.ctx(userAdmin, userOther, ""))
		require.NoError(t, err)
		_, err = f.h.updateUserSettings(f.ctx(userAdmin, userOther, on))
		require.NoError(t, err)
		require.True(t, f.notification(t, userOther))
	})
}

// A user with no workspace, which is every user who holds only the self-read role, reads the own record:
// the one thing that role is for. Reading another's, and listing, are refused: the role names get on the own
// record and nothing else.
func TestSelfReadReadsTheOwnRecordAndNothingMore(t *testing.T) {
	f := newAccessFixture(t)

	for _, name := range []string{common.UserSelf, userSelfRead} {
		res, err := f.h.getUser(f.ctx(userSelfRead, name, ""))
		require.NoError(t, err, name)
		require.NotNil(t, res, name)
	}
	res, err := f.h.getUser(f.ctx(userSelfRead, userOther, ""))
	require.True(t, apierrors.IsForbidden(err), "%v", err)
	require.Nil(t, res)

	// The list is filtered by the same rules: it holds no one, not even the caller (that is get, not list).
	listed, err := f.h.listUser(f.ctx(userSelfRead, "", ""))
	require.NoError(t, err)
	require.Zero(t, listed.(view.ListUserResponse).TotalCount)

	// The workspaces of the record: none, as an empty list.
	ws, err := f.h.getSelfWorkspaces(f.ctx(userSelfRead, "", ""))
	require.NoError(t, err)
	require.Empty(t, ws.(*view.UserWorkspacesResponse).Workspaces)

	// A default user lists the users they share a workspace with (here none) and reads the own record, as ever.
	_, err = f.h.getUser(f.ctx(userDefault, common.UserSelf, ""))
	require.NoError(t, err)
}

// GET /envs is public, and carries the line the console's no-access page shows: the page cannot ask for it,
// because the user it is shown to is refused everything else.
func TestEnvsCarriesTheAccessContact(t *testing.T) {
	t.Cleanup(func() { commonconfig.UnsetValue("console.access_contact") })
	for _, tt := range []struct {
		name  string
		value string
		set   bool
		want  string
	}{
		{"not configured", "", false, ""},
		{"configured", "Ask the platform team", true, "Ask the platform team"},
		{"configured empty", "", true, ""},
		{"surrounding space is dropped", "  Ask  ", true, "Ask"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			commonconfig.UnsetValue("console.access_contact")
			if tt.set {
				commonconfig.SetValue("console.access_contact", tt.value)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil)
			(&Handler{}).GetEnvs(c)
			require.Equal(t, http.StatusOK, w.Code)
			var raw map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
			require.Contains(t, raw, "accessContact", "the field is always present, so a console can tell a server that has it from one that does not")
			require.Equal(t, tt.want, raw["accessContact"])
		})
	}
}
