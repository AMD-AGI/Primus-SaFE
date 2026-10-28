/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The identity of an external request, and what it maps to (extauth.Overlay), lives in the
// context of the request and nowhere else. A check that resolves the requesting user with
// any other context (context.Background(), a context of a background goroutine) finds no
// overlay and sees the stored roles only, so an administrator made so by a group is refused
// there while the same user is allowed one call away. The access controller takes the
// context in AccessInput.Context and in GetRequestUser, and most handlers pass a local
// "ctx"; this test proves that every such context, followed back through assignments and
// through the callers of the functions it is a parameter of, is the request's.

// overlayContextSinks are the calls and fields whose context decides whether the overlay is seen.
var overlayContextSinks = map[string]int{"GetRequestUser": 0, "GetRoles": 0}

type goSource struct {
	fset  *token.FileSet
	files map[string]*ast.File
	// decls and calls index by function name every declaration and every call, over all files.
	decls map[string][]*funcScope
	calls map[string][]*callSite
	// scopes of every function body, declarations and literals alike, innermost last.
	scopes []*funcScope
}

type funcScope struct {
	file  string
	name  string // the declared name; empty for a literal
	typ   *ast.FuncType
	body  *ast.BlockStmt
	outer *funcScope
}

type callSite struct {
	call  *ast.CallExpr
	scope *funcScope
}

func loadHandlerSources(t *testing.T) *goSource {
	t.Helper()
	src := &goSource{fset: token.NewFileSet(), files: map[string]*ast.File{},
		decls: map[string][]*funcScope{}, calls: map[string][]*callSite{}}
	root := ".."
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(src.fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		src.files[path] = f
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			sc := &funcScope{file: path, name: fd.Name.Name, typ: fd.Type, body: fd.Body}
			src.decls[fd.Name.Name] = append(src.decls[fd.Name.Name], sc)
			src.index(sc)
		}
		return nil
	}))
	return src
}

// index records the function scope and, walking its body, the calls it makes and the
// literals nested in it. A call belongs to the innermost function around it.
func (s *goSource) index(sc *funcScope) {
	s.scopes = append(s.scopes, sc)
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			inner := &funcScope{file: sc.file, typ: x.Type, body: x.Body, outer: sc}
			s.index(inner)
			return false
		case *ast.CallExpr:
			if name := calleeName(x); name != "" {
				s.calls[name] = append(s.calls[name], &callSite{call: x, scope: sc})
			}
		}
		return true
	}
	ast.Inspect(sc.body, walk)
}

func calleeName(c *ast.CallExpr) string {
	switch f := c.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func (s *goSource) pos(n ast.Node) string {
	p := s.fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d", strings.TrimPrefix(p.Filename, "../"), p.Line)
}

var derivingCalls = map[string]bool{"WithTimeout": true, "WithCancel": true, "WithDeadline": true,
	"WithValue": true, "WithoutCancel": true, "WithCancelCause": true, "WithTimeoutCause": true}

// fromRequest reports why e is not the context of the request, or "" when it is. An
// identifier is followed to every assignment of it in the function and the functions around it
// and, when it is a parameter, to the argument of every call of the function.
func (s *goSource) fromRequest(e ast.Expr, sc *funcScope, seen map[string]bool) string {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return s.fromRequest(x.X, sc, seen)
	case *ast.CallExpr:
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			// <gin context>.Request.Context()
			if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Request" && sel.Sel.Name == "Context" {
				return ""
			}
			// context.WithTimeout(parent, ...) and friends: as fresh as the parent.
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "context" {
				if derivingCalls[sel.Sel.Name] && len(x.Args) > 0 {
					return s.fromRequest(x.Args[0], sc, seen)
				}
				if sel.Sel.Name == "Background" || sel.Sel.Name == "TODO" {
					return "context." + sel.Sel.Name + "() is not the request's context: " + s.pos(x)
				}
			}
		}
		return "a call this test does not know: " + s.pos(x)
	case *ast.Ident:
		return s.identFromRequest(x, sc, seen)
	case *ast.SelectorExpr:
		// The access controller reading the context its caller put in AccessInput: what the
		// callers put there is checked at each AccessInput literal.
		if recv, ok := x.X.(*ast.Ident); ok && recv.Name == "in" && x.Sel.Name == "Context" &&
			strings.HasSuffix(sc.file, "authority/access_controller.go") {
			return ""
		}
	}
	return fmt.Sprintf("an expression this test does not know (%T): %s", e, s.pos(e))
}

func (s *goSource) identFromRequest(id *ast.Ident, sc *funcScope, seen map[string]bool) string {
	for fn := sc; fn != nil; fn = fn.outer {
		// Assignments in this function body (not in literals nested in it).
		var rhss []ast.Expr
		defined := false
		ast.Inspect(fn.body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.AssignStmt:
				for i, l := range x.Lhs {
					if li, ok := l.(*ast.Ident); ok && li.Name == id.Name {
						defined = true
						switch {
						case len(x.Rhs) == len(x.Lhs):
							rhss = append(rhss, x.Rhs[i])
						case len(x.Rhs) == 1:
							rhss = append(rhss, x.Rhs[0])
						}
					}
				}
			case *ast.ValueSpec:
				for i, nm := range x.Names {
					if nm.Name == id.Name {
						defined = true
						if i < len(x.Values) {
							rhss = append(rhss, x.Values[i])
						} else if len(x.Values) == 1 {
							rhss = append(rhss, x.Values[0])
						}
					}
				}
			}
			return true
		})
		if defined {
			if len(rhss) == 0 {
				return "assigned in a way this test does not know: " + s.pos(id)
			}
			for _, r := range rhss {
				if why := s.fromRequest(r, fn, seen); why != "" {
					return why
				}
			}
			return ""
		}
		// A parameter of this function.
		if idx := paramIndex(fn.typ, id.Name); idx >= 0 {
			return s.paramFromRequest(fn, idx, id, seen)
		}
	}
	return fmt.Sprintf("%s is declared somewhere this test cannot see: %s", id.Name, s.pos(id))
}

func paramIndex(ft *ast.FuncType, name string) int {
	i := 0
	for _, f := range ft.Params.List {
		for _, n := range f.Names {
			if n.Name == name {
				return i
			}
			i++
		}
		if len(f.Names) == 0 {
			i++
		}
	}
	return -1
}

// paramFromRequest requires every call of the declared function to pass the request's context.
func (s *goSource) paramFromRequest(fn *funcScope, idx int, id *ast.Ident, seen map[string]bool) string {
	if fn.name == "" {
		return fmt.Sprintf("%s is a parameter of a function literal, whose callers this test cannot follow: %s", id.Name, s.pos(id))
	}
	key := fmt.Sprintf("%s#%d", fn.name, idx)
	if seen[key] {
		return ""
	}
	seen[key] = true
	sites := s.calls[fn.name]
	if len(sites) == 0 {
		return fmt.Sprintf("%s takes %s as a parameter and nothing in the tree calls it: %s", fn.name, id.Name, s.pos(id))
	}
	for _, site := range sites {
		if idx >= len(site.call.Args) {
			continue // another function with the same name and a shorter signature
		}
		if why := s.fromRequest(site.call.Args[idx], site.scope, seen); why != "" {
			return fmt.Sprintf("%s (passed to %s at %s)", why, fn.name, s.pos(site.call))
		}
	}
	return ""
}

// notRequestContexts are the places that authorize with a context that is not an HTTP request's,
// because there is no HTTP request: each is named with why that is so. An entry that no
// longer matches a finding fails the test, so the list cannot outlive the code it excuses.
var notRequestContexts = map[string]string{
	"ssh-handlers/k8s.go": "an SSH session is authenticated by public key on the SSH port; no external authenticator " +
		"ran, so no overlay exists for it, and the roles of its user are the ones stored",
}

func TestEveryAuthorizationContextIsTheRequestContext(t *testing.T) {
	src := loadHandlerSources(t)
	checked := 0
	var bad []string
	excused := map[string]bool{}
	check := func(e ast.Expr, sc *funcScope, what string) {
		checked++
		why := src.fromRequest(e, sc, map[string]bool{})
		if why == "" {
			return
		}
		if _, ok := notRequestContexts[sc.file[len("../"):]]; ok {
			excused[sc.file[len("../"):]] = true
			return
		}
		bad = append(bad, fmt.Sprintf("%s at %s: %s", what, src.pos(e), why))
	}
	for _, sc := range src.scopes {
		ast.Inspect(sc.body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncLit:
				return false // visited as a scope of its own
			case *ast.CompositeLit:
				if isAccessInput(x.Type) {
					for _, el := range x.Elts {
						if kv, ok := el.(*ast.KeyValueExpr); ok {
							if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Context" {
								check(kv.Value, sc, "AccessInput.Context")
							}
						}
					}
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if idx, isSink := overlayContextSinks[sel.Sel.Name]; isSink && len(x.Args) > idx && isAccessController(sel.X) {
						check(x.Args[idx], sc, sel.Sel.Name)
					}
				}
			}
			return true
		})
	}
	sort.Strings(bad)
	for file := range notRequestContexts {
		require.True(t, excused[file], "%s is excused but nothing in it needs the excuse any more: remove it", file)
	}
	t.Logf("checked %d contexts", checked)
	require.Greater(t, checked, 30, "the scan must see the handlers' call sites, or it proves nothing")
	require.Empty(t, bad, "a context that is not the request's cannot carry the overlay of an external request:\n%s", strings.Join(bad, "\n"))
}

func isAccessInput(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "AccessInput"
	case *ast.Ident:
		return x.Name == "AccessInput"
	}
	return false
}

// isAccessController reports whether the receiver looks like the access controller, so a
// GetRoles of something else does not count. The tree names it accessController everywhere
// except inside the controller itself, where it is the receiver "a".
func isAccessController(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "accessController" || x.Sel.Name == "AccessController"
	case *ast.Ident:
		return x.Name == "a" || x.Name == "ac" || x.Name == "accessController"
	}
	return false
}
