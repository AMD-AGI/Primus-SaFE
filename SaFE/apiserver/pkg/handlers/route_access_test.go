/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority/authtest"
	cdhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/cd-handlers"
	emailrelayhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/email-relay-handlers"
	imagehandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/image-handlers"
	inferencexhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/inferencex"
	lenscompat "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/lens-compat"
	llmgateway "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/llm-gateway"
	model_handlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/model-handlers"
	optimizationhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/optimization"
	proxyhandlers "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/proxy-handlers"
	reshandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources"
	sshhandler "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/ssh-handlers"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

// The sweep: every route the apiserver registers, served to a user who holds no access, to a user who
// holds the default role and to an administrator. Handlers here have nothing behind them (they are
// the zero value), which is the point: a user who holds no access must be refused before any
// handler runs, so a refusal that comes from a handler's own check, or a handler that was
// never reached for want of a database, proves nothing. The routes are those registerRoutes
// registers, the one list InitHttpHandlers serves, so a route that is added is swept without
// anyone adding it here.

// sweepHandlers is every handler registerRoutes takes, each the zero value and each with its dependencies
// switched on, so that every route it can register is registered.
func sweepHandlers(t *testing.T) routeHandlers {
	t.Helper()
	viper.Set("proxy.services", []map[string]any{{"name": "sweep", "prefix": "/sweep-proxy", "target": "http://127.0.0.1:9", "enabled": true}})
	t.Cleanup(func() { viper.Set("proxy.services", nil) })
	proxy, err := proxyhandlers.NewProxyHandler()
	require.NoError(t, err)
	return routeHandlers{
		proxy:          proxy,
		lens:           lenscompat.NewHandler(&robustclient.Client{}),
		resources:      &reshandler.Handler{},
		cd:             &cdhandlers.Handler{},
		image:          &imagehandlers.ImageHandler{},
		ssh:            &sshhandler.SshHandler{},
		model:          &model_handlers.Handler{},
		llm:            &llmgateway.Handler{},
		emailRelay:     &emailrelayhandlers.Handler{},
		githubWorkflow: true,
		inferencex:     inferencexhandlers.NewHandler(time.Hour),
		optimization:   &optimizationhandlers.Handler{},
		mcp:            true,
		scimToken:      "sweep-token",
	}
}

var (
	routeParam = regexp.MustCompile(`[:*][A-Za-z]+`)
	// handlerPackage is the package directory of a handler as gin names it:
	// ".../apiserver/pkg/handlers/cd-handlers.(*Handler).Create-fm" is "handlers/cd-handlers".
	handlerPackage = regexp.MustCompile(`/apiserver/pkg/([^.]+)\.`)
)

type sweepEnv struct {
	engine *gin.Engine
	tokens map[string]string
}

func newSweepEnv(t *testing.T) *sweepEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	commonconfig.SetValue("crypto.enable", "false")
	t.Cleanup(func() { commonconfig.SetValue("crypto.enable", "") })

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	mk := func(name string, roles ...v1.UserRole) *v1.User {
		id := commonuser.GenerateUserIdByName(name)
		return &v1.User{
			ObjectMeta: metav1.ObjectMeta{Name: id, Labels: map[string]string{v1.UserIdLabel: id}, Annotations: map[string]string{v1.UserNameAnnotation: name}},
			Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: roles, Password: stringutil.Base64Encode("pw")},
		}
	}
	objs := append(authtest.ChartRoles(t),
		mk("no-access-user", v1.SelfReadRole), mk("plain-user", v1.DefaultRole), mk("admin-user", v1.SystemAdminRole))
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	tok := authority.NewDefaultToken(cli)
	require.NotNil(t, tok)
	require.True(t, tok.Client == cli, "the sign-in is bound to another cluster: this test must be the first to bind it")

	e := gin.New()
	registerRoutes(e, sweepHandlers(t))
	env := &sweepEnv{engine: e, tokens: map[string]string{}}
	for _, name := range []string{"no-access-user", "plain-user", "admin-user"} {
		_, resp, err := tok.Login(t.Context(), authority.TokenInput{Username: name, Password: "pw"})
		require.NoError(t, err)
		env.tokens[name] = resp.Token
	}
	return env
}

// call serves one request; a handler that panics (a zero handler that was reached) answers 599.
func (e *sweepEnv) call(r gin.RouteInfo, token string) (code int, body string) {
	path := routeParam.ReplaceAllString(r.Path, "x")
	// A route may hold the connection open (the MCP event stream does): the request gives up on its own.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(r.Method, path, strings.NewReader("{}")).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := &syncRecorder{ResponseRecorder: httptest.NewRecorder()}
	func() {
		defer func() {
			if recover() != nil {
				w.mu.Lock()
				w.Code = 599
				w.mu.Unlock()
			}
		}()
		e.engine.ServeHTTP(w, req)
	}()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Code, w.Body.String()
}

// syncRecorder is a response recorder that a goroutine the handler started (the MCP event stream keeps a
// heartbeat going) can write to while the test reads it.
type syncRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func (r *syncRecorder) Header() http.Header {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Header()
}

func (r *syncRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Write(b)
}

func (r *syncRecorder) WriteString(s string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.WriteString(s)
}

func (r *syncRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ResponseRecorder.WriteHeader(code)
}

func (r *syncRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ResponseRecorder.Flush()
}

func refusedByTheGate(code int, body string) bool {
	return code == http.StatusForbidden && strings.Contains(body, authority.ErrNoAccess) &&
		strings.Contains(body, commonerrors.NoAccess)
}

// Routes the apiserver serves without authenticating the caller. A route that appears here without
// being reviewed is a route that serves anyone, including a user who holds no access, so the list is
// of the exceptions and the test fails for a new one.
var publicRoutes = map[string]string{
	"POST /api/v1/login":                       "signs in",
	"POST /api/v1/users":                       "registers (refused when single sign-on is on)",
	"GET /api/v1/envs":                         "the sign-in page needs it before anyone is signed in",
	"POST /api/v1/auth/verify":                 "services call it with the internal token",
	"POST /api/v1/auditlogs":                   "services report operations with the internal token (checked in the handler)",
	"PUT /api/v1/images:import/:name/progress": "the image import job reports progress (checked in the handler)",
}

func TestEveryAuthenticatedRouteRefusesAUserWhoHoldsNoAccess(t *testing.T) {
	env := newSweepEnv(t)
	routes := env.engine.Routes()
	require.Greater(t, len(routes), 150, "the route table is far smaller than the apiserver's: a registration was lost")

	var public, authenticated []gin.RouteInfo
	packages := map[string]int{}
	for _, r := range routes {
		if m := handlerPackage.FindStringSubmatch(r.Handler); m != nil {
			packages[m[1]]++
		} else {
			packages["(unnamed) "+r.Handler]++
		}
		// An authenticated route answers a request with no credential with 401.
		if code, _ := env.call(r, ""); code == http.StatusUnauthorized {
			authenticated = append(authenticated, r)
		} else {
			public = append(public, r)
		}
	}

	// 1. The routes that do not authenticate are exactly the reviewed ones.
	var gotPublic []string
	for _, r := range public {
		gotPublic = append(gotPublic, r.Method+" "+r.Path)
	}
	var wantPublic []string
	for k := range publicRoutes {
		wantPublic = append(wantPublic, k)
	}
	// The MCP transport is not authenticated by the apiserver: the tools forward the caller's own
	// credentials to the routes above, which are.
	gotNoMCP := gotPublic[:0:0]
	for _, p := range gotPublic {
		if !strings.Contains(p, "/safe-mcp/") {
			gotNoMCP = append(gotNoMCP, p)
		}
	}
	sort.Strings(gotNoMCP)
	sort.Strings(wantPublic)
	require.Equal(t, wantPublic, gotNoMCP, "the routes that serve a caller who is not signed in changed: review the new route, "+
		"and list it in publicRoutes only if serving anyone is right for it")

	// 2. The packages the routes come from are the ones the apiserver is made of: a package that registers
	// routes and is neither here nor swept would be a hole in this test.
	var gotPackages []string
	for p := range packages {
		gotPackages = append(gotPackages, p)
	}
	sort.Strings(gotPackages)
	require.Equal(t, []string{
		"handlers/authority", "handlers/cd-handlers", "handlers/email-relay-handlers", "handlers/github-workflow",
		"handlers/image-handlers", "handlers/inferencex", "handlers/lens-compat", "handlers/llm-gateway",
		"handlers/model-handlers", "handlers/optimization", "handlers/proxy-handlers", "handlers/resources",
		"handlers/scim", "handlers/ssh-handlers", "mcp/router",
	}, gotPackages, "the packages that register routes changed: check that registerRoutes still registers them all")

	// 3. A user who holds no access is refused by the gate everywhere but on the routes it is served.
	var served []string
	for _, r := range authenticated {
		// SCIM authenticates its own bearer. A user session is not that token, so the gate does not run.
		if strings.HasPrefix(r.Path, "/scim/") {
			continue
		}
		code, body := env.call(r, env.tokens["no-access-user"])
		if authority.IsNoAccessRoute(r.Method, r.Path) {
			served = append(served, r.Method+" "+r.Path)
			require.False(t, refusedByTheGate(code, body), "%s %s must be served to a user who holds no access: %d %s", r.Method, r.Path, code, body)
			continue
		}
		require.True(t, refusedByTheGate(code, body),
			"%s %s was not refused by the gate for a user who holds no access: %d %s", r.Method, r.Path, code, body)
	}
	sort.Strings(served)
	require.Equal(t, []string{"GET /api/v1/users/:name", "GET /api/v1/users/self/workspaces", "POST /api/v1/logout"}, served,
		"the routes served to a user who holds no access are not the three own-record routes")

	// 4. The gate costs everyone else nothing: no route is refused by it for a user who holds access.
	for _, who := range []string{"plain-user", "admin-user"} {
		for _, r := range authenticated {
			code, body := env.call(r, env.tokens[who])
			require.False(t, refusedByTheGate(code, body), "%s was refused %s %s by the gate", who, r.Method, r.Path)
		}
	}
	t.Logf("swept %d routes: %d authenticated, %d public, %d served to a user who holds no access",
		len(routes), len(authenticated), len(public), len(served))
}
