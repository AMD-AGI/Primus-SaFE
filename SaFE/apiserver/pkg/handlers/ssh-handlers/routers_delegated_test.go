/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ssh_handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/extauth"
)

var routeParam = regexp.MustCompile(`:[A-Za-z]+`)

// upgradingHandlers returns the names of the functions of this package that upgrade the
// connection: those that call an Upgrade or Hijack method, and, transitively, those
// that call one of them. It reads the source, so it follows the code and not a list.
func upgradingHandlers(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	require.NoError(t, err)

	calls := map[string]map[string]bool{} // function name -> names it calls
	upgrading := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				if calls[fn.Name.Name] == nil {
					calls[fn.Name.Name] = map[string]bool{}
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					switch f := call.Fun.(type) {
					case *ast.SelectorExpr:
						if f.Sel.Name == "Upgrade" || f.Sel.Name == "Hijack" {
							upgrading[fn.Name.Name] = true
						}
						calls[fn.Name.Name][f.Sel.Name] = true
					case *ast.Ident:
						calls[fn.Name.Name][f.Name] = true
					}
					return true
				})
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for fn, callees := range calls {
			if upgrading[fn] {
				continue
			}
			for callee := range callees {
				if upgrading[callee] {
					upgrading[fn], changed = true, true
					break
				}
			}
		}
	}
	return upgrading
}

// handlerName turns gin's "pkg/path.(*Type).Method-fm" into "Method".
func handlerName(h string) string {
	h = strings.TrimSuffix(h, "-fm")
	return h[strings.LastIndex(h, ".")+1:]
}

// Every route of this package whose handler upgrades the connection (the web shell is a
// GET into an interactive exec session) must be classified as interactive, so that a
// delegated token limited to safe.read cannot use it. The routes are found by reading
// the handlers, so a route added later is held to the same check.
func TestUpgradeRoutesAreInteractiveForDelegatedTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	InitWebShellRouters(e, &SshHandler{})

	upgrading := upgradingHandlers(t)
	isUpgrade := func(h string) bool { return upgrading[handlerName(h)] }
	require.Empty(t, extauth.InteractiveRouteGaps(e.Routes(), isUpgrade),
		"a route upgrades the connection but is not classified in extauth.Interactive")

	seen := 0
	for _, r := range e.Routes() {
		if !isUpgrade(r.Handler) {
			continue
		}
		seen++
		path := routeParam.ReplaceAllString(r.Path, "x")
		for _, headers := range []http.Header{nil, {"Connection": {"Upgrade"}, "Upgrade": {"websocket"}}} {
			req := httptest.NewRequest(r.Method, path, nil)
			for k, v := range headers {
				req.Header[k] = v
			}
			require.False(t, extauth.ScopeAllows([]string{extauth.ScopeRead}, req),
				"%s %s must not be allowed for safe.read (headers %v)", r.Method, r.Path, headers)
			require.True(t, extauth.ScopeAllows([]string{extauth.ScopeAPI}, req),
				"%s %s stays available with safe.api", r.Method, r.Path)
		}
	}
	require.Positive(t, seen, "no upgrading route found: the test would pass vacuously")
	require.True(t, upgrading["WebShell"], "the scan must see the web shell handler upgrade")
}

// No other package may serve WebSocket connections without a route-table test like the
// one above: this fails when a package outside the list starts importing a WebSocket
// library, so that its routes get classified.
func TestOnlyReviewedPackagesServeWebSockets(t *testing.T) {
	root, err := filepath.Abs("../..") // apiserver/pkg
	require.NoError(t, err)
	found := map[string]bool{}
	require.NoError(t, filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if perr != nil {
			return perr
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if strings.Contains(path, "websocket") {
				rel, _ := filepath.Rel(root, filepath.Dir(p))
				found[filepath.ToSlash(rel)] = true
			}
		}
		return nil
	}))
	var got []string
	for p := range found {
		got = append(got, p)
	}
	sort.Strings(got)
	require.Equal(t, []string{"handlers/ssh-handlers"}, got,
		"a package serves WebSocket connections: add a route-table test that calls extauth.InteractiveRouteGaps for its routes, then list it here")
}
