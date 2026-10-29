/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// What identity an authorization sees depends on how the session was authenticated:
//   - the web shell is an HTTP request the identity middleware (ParseToken) authenticated, and its
//     context already carries the overlay of the identity provider's roles and workspaces (or, for
//     a built-in credential in external mode, the overlay of a user no identity vouched for);
//   - an SSH session (shell, SFTP, direct-tcpip, remote forward) is authenticated by public key on
//     the SSH port, so in external mode its user is one no identity vouched for, and the platform
//     roles stored on the user object do not count.
//
// The tests below run every path against the same users in both modes, through the entry point of
// each path, and watch whether the request got past authorization to the pod.

const (
	authTestNamespace = "ns-auth"
	authTestCluster   = "c-auth"
	authTestWorkspace = "ws-a"
	authTestPod       = "pod-0"
)

// authTestMappings are the auth.group_mappings of the tests: a group that grants a workspace and
// a group that grants system-admin.
var authTestMappings = []commonconfig.AuthGroupMapping{
	{Group: "team-a", Roles: []string{string(v1.DefaultRole)}, Workspaces: []string{authTestWorkspace}},
	{Group: "admins", Roles: []string{string(v1.SystemAdminRole)}},
}

// authFixture is a handler whose workspace, pod and workload resolve against a small API server,
// so a request that passes authorization visibly reaches the pod.
type authFixture struct {
	h       *SshHandler
	wsURL   string
	reached atomic.Int32 // requests that got past authorization (an exec, a port-forward, a session record)
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &authFixture{}

	// The cluster of the pod: it answers the pod lookup and counts what reaches the pod.
	podSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/"+authTestNamespace+"/pods/"+authTestPod:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(&corev1.Pod{
				TypeMeta: metav1.TypeMeta{Kind: "Pod", APIVersion: "v1"},
				ObjectMeta: metav1.ObjectMeta{Name: authTestPod, Namespace: authTestNamespace,
					Labels: map[string]string{v1.WorkloadIdLabel: "wl-1"}},
			})
		case r.Method == http.MethodPost &&
			(strings.HasSuffix(r.URL.Path, "/exec") || strings.HasSuffix(r.URL.Path, "/portforward")):
			f.reached.Add(1)
			http.Error(w, "there is no pod behind this test server", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(podSrv.Close)
	restConfig := &rest.Config{Host: podSrv.URL}
	clientSet, err := kubernetes.NewForConfig(restConfig)
	require.NoError(t, err)
	factory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), authTestCluster, clientSet)
	factory.AttachRestConfigForTest(restConfig)
	clientManager := commonutils.NewObjectManager()
	require.NoError(t, clientManager.Add(authTestCluster, factory))

	user := func(name string, roles ...v1.UserRole) *v1.User {
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{v1.UserIdLabel: name}},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles},
		}
	}
	// The default role reaches the workloads of its owner and of the workspaces the user holds.
	defaultRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.DefaultRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.GetVerb},
			GrantedUsers: []string{authority.GrantedOwner, authority.GrantedWorkspaceUser},
		}},
	}
	adminRole := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: string(v1.SystemAdminRole)},
		Rules: []v1.PolicyRule{{
			Resources:    []string{authority.AllResource},
			Verbs:        []v1.RoleVerb{v1.AllVerb},
			GrantedUsers: []string{authority.GrantedAllUser},
		}},
	}
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: authTestNamespace},
		Spec:       v1.WorkspaceSpec{Cluster: authTestCluster},
	}
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1", Labels: map[string]string{v1.UserIdLabel: "owner"}},
		Spec:       v1.WorkloadSpec{Workspace: authTestWorkspace},
	}
	cli := ctrlfake.NewClientBuilder().WithScheme(sshScheme(t)).WithObjects(
		workspace, workload, defaultRole, adminRole,
		user("stored-admin", v1.SystemAdminRole),
		user("owner", v1.DefaultRole),
		user("member", v1.DefaultRole),
		user("nobody", v1.DefaultRole),
	).Build()

	ctrl := gomock.NewController(t)
	db := mock_client.NewMockInterface(ctrl)
	// A session that gets past authorization stops here, at the record of it.
	db.EXPECT().InsertSshSessionRecord(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *dbclient.SshSessionRecords) (int64, error) {
			f.reached.Add(1)
			return 0, errors.New("the test stops a session at its record")
		}).AnyTimes()

	f.h = &SshHandler{
		Client:        cli,
		dbClient:      db,
		clientManager: clientManager,
		// Not NewAccessController: that is a singleton, and another test's client would win.
		accessController: &authority.AccessController{Client: cli},
		timeout:          time.Minute,
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}

	// The web shell route, behind what ParseToken leaves on the request: the user, and in
	// external mode the overlay of the identity.
	router := gin.New()
	router.GET("/workloads/:name/pods/:"+common.PodId+"/webshell", func(c *gin.Context) {
		u := c.GetHeader("X-Test-User")
		c.Set(common.UserId, u)
		var groups []string
		if g := c.GetHeader("X-Test-Groups"); g != "" {
			groups = strings.Split(g, ",")
		}
		c.Request = c.Request.WithContext(requestContext(c.Request.Context(), u, c.GetHeader("X-Test-Identity"), groups))
		c.Next()
	}, f.h.WebShell)
	webSrv := httptest.NewServer(router)
	t.Cleanup(webSrv.Close)
	f.wsURL = "ws" + strings.TrimPrefix(webSrv.URL, "http") +
		"/workloads/wl-1/pods/" + authTestPod + "/webshell?namespace=" + authTestNamespace
	return f
}

// requestContext is the context ParseToken leaves on a request. identity is "" in builtin mode,
// where it adds nothing; in external mode it is "idp" for a request an identity provider vouched
// for, whose groups the mappings turn into roles and workspaces, and "builtin" for a built-in
// session or API key, which no identity vouched for.
func requestContext(ctx context.Context, userId, identity string, groups []string) context.Context {
	switch identity {
	case "idp":
		return extauth.WithOverlay(ctx, extauth.MapGroups(userId, groups, authTestMappings, false))
	case "builtin":
		return authority.WithoutIdentity(ctx, userId)
	}
	return ctx
}

// setAuthMode puts the apiserver in builtin or external mode for the test.
func setAuthMode(t *testing.T, mode string) {
	t.Helper()
	if mode != "external" {
		return
	}
	viper.Set("auth.mode", "external")
	viper.Set("auth.authenticators", []map[string]any{{"type": "builtin"}})
	require.NoError(t, authority.InitExternalAuth(nil))
	t.Cleanup(func() {
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		require.NoError(t, authority.InitExternalAuth(nil))
	})
}

// authCase is a user and the identity of their web shell request in external mode, with whether
// each path lets them in, per mode. In builtin mode there is no identity: the stored record decides.
type authCase struct {
	name     string
	user     string
	identity string   // external mode, web shell: "idp" or "builtin"
	groups   []string // external mode, web shell: the groups of the identity
	// want is whether the user reaches the pod: [builtin web shell, builtin SSH, external web shell,
	// external SSH]
	want [4]bool
}

var authCases = []authCase{
	{
		// An administrator by the stored record alone: the identity provider never granted it.
		name: "stored system-admin, no identity provider", user: "stored-admin", identity: "builtin",
		want: [4]bool{true, true, false, false},
	},
	{
		// The identity provider took the admin group away and the record still lists the role.
		name: "stored system-admin whose group was removed", user: "stored-admin", identity: "idp", groups: []string{},
		want: [4]bool{true, true, false, false},
	},
	{
		// The role stored on the record is what the group grants: the web shell must see it.
		name: "system-admin stored and granted by group", user: "stored-admin", identity: "idp", groups: []string{"admins"},
		want: [4]bool{true, true, true, false},
	},
	{
		// The scenario of the review: a workspace only the identity provider grants.
		name: "workspace granted by a group", user: "member", identity: "idp", groups: []string{"team-a"},
		want: [4]bool{false, false, true, false},
	},
	{
		// The other scenario of the review: system-admin only the identity provider grants.
		name: "system-admin granted by a group", user: "member", identity: "idp", groups: []string{"admins"},
		want: [4]bool{false, false, true, false},
	},
	{
		// The account is kept: only the platform role went, not ownership of the workload.
		name: "owner of the workload", user: "owner", identity: "builtin",
		want: [4]bool{true, true, true, true},
	},
	{
		name: "unrelated user", user: "nobody", identity: "idp", groups: []string{},
		want: [4]bool{false, false, false, false},
	},
}

// authPath is one way in to a pod. It returns whether the request got past authorization, and the
// error the path itself reports, when it reports one.
type authPath struct {
	name string
	// web is whether the path is the web shell, which is authorized as the request's identity;
	// every other path is an SSH one.
	web bool
	run func(t *testing.T, f *authFixture, c authCase, mode string) (reached bool, err error)
}

func sshLogin(user string) string {
	return user + "." + authTestPod + ".main.bash." + authTestNamespace
}

func (f *authFixture) userInfo(user string) *UserInfo {
	return &UserInfo{User: user, Pod: authTestPod, Container: "main", CMD: "bash", Namespace: authTestNamespace}
}

var authPaths = []authPath{
	{name: "web shell", web: true, run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		hdr := http.Header{}
		hdr.Set("X-Test-User", c.user)
		if mode == "external" {
			hdr.Set("X-Test-Identity", c.identity)
			hdr.Set("X-Test-Groups", strings.Join(c.groups, ","))
		}
		before := f.reached.Load()
		conn, _, err := websocket.DefaultDialer.Dial(f.wsURL, hdr)
		require.NoError(t, err)
		defer conn.Close()
		// The handler closes the socket once the session has ended, which is after authorization.
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				require.False(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "i/o timeout"),
					"the web shell did not finish")
				break
			}
		}
		return f.reached.Load() > before, nil
	}},
	{name: "ssh session", run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		before := f.reached.Load()
		f.h.handleSession(newSessionStub(sshLogin(c.user)))
		return f.reached.Load() > before, nil
	}},
	{name: "ssh sftp", run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		before := f.reached.Load()
		f.h.handleSftp(newSessionStub(sshLogin(c.user)))
		return f.reached.Load() > before, nil
	}},
	{name: "ssh direct-tcpip", run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		before := f.reached.Load()
		nc := &fakeNewChannel{extraData: ssh.Marshal(forwardChannelData{
			DestAddr: "127.0.0.1", DestPort: 80, OriginAddr: "127.0.0.1", OriginPort: 8080})}
		done := make(chan struct{})
		go func() {
			defer close(done)
			f.h.handleDirectIp(context.Background(), &ssh.ServerConn{Conn: fakeSSHConn{user: sshLogin(c.user)}}, nc)
		}()
		// A refused request returns at once; one that passes waits for the port-forward to come up,
		// which it never does against this server, so it is seen by the request reaching the pod.
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) && f.reached.Load() == before {
			select {
			case <-done:
				return f.reached.Load() > before, nil
			case <-time.After(10 * time.Millisecond):
			}
		}
		return f.reached.Load() > before, nil
	}},
	{name: "ssh remote forward", run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		_, err := f.h.resolveForwardTarget(context.Background(), f.userInfo(c.user))
		return err == nil, err
	}},
	{name: "authUser, the default", run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		workload := &v1.Workload{}
		require.NoError(t, f.h.Get(context.Background(), clientKey("wl-1"), workload))
		err := f.h.authUser(context.Background(), f.userInfo(c.user), workload)
		return err == nil, err
	}},
	{name: "authUserAsRequestIdentity", web: true, run: func(t *testing.T, f *authFixture, c authCase, mode string) (bool, error) {
		workload := &v1.Workload{}
		require.NoError(t, f.h.Get(context.Background(), clientKey("wl-1"), workload))
		identity := ""
		if mode == "external" {
			identity = c.identity
		}
		ctx := requestContext(context.Background(), c.user, identity, c.groups)
		err := f.h.authUserAsRequestIdentity(ctx, f.userInfo(c.user), workload)
		return err == nil, err
	}},
}

// Both modes, every user, every path: the web shell holds what the identity provider granted and
// the SSH paths hold what a built-in credential holds.
func TestAuthorizationByPathAndMode(t *testing.T) {
	for _, mode := range []string{"builtin", "external"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			setAuthMode(t, mode)
			f := newAuthFixture(t)
			for _, c := range authCases {
				for _, p := range authPaths {
					want := c.want[0]
					switch {
					case mode == "builtin" && !p.web:
						want = c.want[1]
					case mode == "external" && p.web:
						want = c.want[2]
					case mode == "external":
						want = c.want[3]
					}
					t.Run(c.name+"/"+p.name, func(t *testing.T) {
						reached, err := p.run(t, f, c, mode)
						require.Equal(t, want, reached, "reaches the pod (error: %v)", err)
						if !want && err != nil {
							// A refusal is the authorization's, not a lookup that failed.
							require.True(t, commonerrors.IsForbidden(err), "refused for another reason: %v", err)
						}
					})
				}
			}
		})
	}
}

// The identity provider's grant is not lost on the web shell, and not borrowed by SSH: one user,
// one group, the two paths, and the record never changes.
func TestWebShellKeepsIdentityOverlayButSSHDoesNot(t *testing.T) {
	setAuthMode(t, "external")
	f := newAuthFixture(t)
	c := authCase{user: "member", identity: "idp", groups: []string{"team-a"}}
	web, _ := authPaths[0].run(t, f, c, "external")
	require.True(t, web, "the web shell keeps the workspace the group grants")
	viaSSH, _ := authPaths[1].run(t, f, c, "external")
	require.False(t, viaSSH, "an SSH session has no identity provider behind it")
}

// authUser is the restrictive function, so an entry path added later that calls it as it is, with
// whatever context it has, gets the restrictive behaviour: in external mode the roles stored on the
// user object do not count, and an overlay in ctx (here one that grants system-admin) is not
// honoured either, because only authUserAsRequestIdentity, which the web shell alone calls, opts in
// to it. In builtin mode the stored record decides, as it always did.
func TestAuthUserDefaultIsRestrictiveForANewCaller(t *testing.T) {
	grantedByIdentity := func() context.Context {
		return extauth.WithOverlay(context.Background(),
			extauth.MapGroups("member", []string{"admins"}, authTestMappings, false))
	}
	for _, tc := range []struct {
		name string
		mode string
		user string
		ctx  func() context.Context
		want bool
	}{
		// What a new SSH path hands it: the connection's context, with no overlay in it.
		{"external: stored system-admin, no overlay in ctx", "external", "stored-admin", context.Background, false},
		{"external: system-admin only an overlay grants", "external", "member", grantedByIdentity, false},
		{"external: owner of the workload", "external", "owner", context.Background, true},
		{"builtin: stored system-admin", "builtin", "stored-admin", context.Background, true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			setAuthMode(t, tc.mode)
			f := newAuthFixture(t)
			workload := &v1.Workload{}
			require.NoError(t, f.h.Get(context.Background(), clientKey("wl-1"), workload))
			err := f.h.authUser(tc.ctx(), f.userInfo(tc.user), workload)
			if tc.want {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.True(t, commonerrors.IsForbidden(err), "refused for another reason: %v", err)
			// The opt-in is what differs: with the identity's overlay, authUserAsRequestIdentity lets
			// the user in that authUser refuses, so the refusal above is the default's, not the fixture's.
			if tc.name == "external: system-admin only an overlay grants" {
				require.NoError(t, f.h.authUserAsRequestIdentity(tc.ctx(), f.userInfo(tc.user), workload))
			}
		})
	}
}

func clientKey(name string) client.ObjectKey { return client.ObjectKey{Name: name} }
