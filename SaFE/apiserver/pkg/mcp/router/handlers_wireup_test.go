/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package router

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHandlersGo_WiresMCPRouter locks down the MCP wire-up inside the
// apiserver's handlers.go so the regression introduced by PR #511 (merge
// commit 3175930d, which silently dropped the mcprouter import + InitRoutes
// call when merging an out-of-date branch back into main) cannot reoccur.
//
// The follow-up fix 73834fa4 "restore MCP config..." only restored the
// commonconfig helpers (IsMCPEnable, GetMCPBasePath, ...) but missed the
// call site in handlers.go, leaving every freshly built apiserver image
// silently shipping without /api/v1/mcp routes despite mcp.enabled=true.
//
// The test lives here (not in pkg/handlers) so it can run without pulling in
// the apiserver's containers/storage dependency tree (libbtrfs-dev). It uses
// go/ast static analysis of handlers.go to assert:
//  1. the mcprouter import is present,
//  2. InitHttpHandlers checks commonconfig.IsMCPEnable(),
//  3. InitHttpHandlers calls mcprouter.InitRoutes(engine) inside that guard.
func TestHandlersGo_WiresMCPRouter(t *testing.T) {
	const (
		mcpRouterImport = "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/mcp/router"
		funcName        = "InitHttpHandlers"
		gateFn          = "IsMCPEnable"
		initFn          = "InitRoutes"
	)

	handlersPath := locateHandlersGo(t)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, handlersPath, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", handlersPath, err)
	}

	var (
		hasMCPImport bool
		mcpAlias     string
	)
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == mcpRouterImport {
			hasMCPImport = true
			if imp.Name != nil {
				mcpAlias = imp.Name.Name
			} else {
				mcpAlias = "router"
			}
			break
		}
	}
	if !hasMCPImport {
		t.Fatalf("handlers.go must import %q (regression: PR #511 dropped it; restore the mcprouter import)", mcpRouterImport)
	}

	var initFunc *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name.Name == funcName {
			initFunc = fn
			break
		}
	}
	if initFunc == nil {
		t.Fatalf("function %s not found in handlers.go", funcName)
	}

	var (
		sawGate    bool
		initInGate bool
	)
	ast.Inspect(initFunc, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok {
			return true
		}
		if !isCallSelector(ifStmt.Cond, "commonconfig", gateFn) {
			return true
		}
		sawGate = true
		ast.Inspect(ifStmt.Body, func(inner ast.Node) bool {
			call, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isCallSelector(call, mcpAlias, initFn) {
				initInGate = true
				return false
			}
			return true
		})
		return true
	})

	if !sawGate {
		t.Errorf("InitHttpHandlers must guard MCP wire-up with commonconfig.%s()", gateFn)
	}
	if !initInGate {
		t.Errorf("InitHttpHandlers must call %s.%s(engine) inside the commonconfig.%s() gate (regression: PR #511 dropped it)", mcpAlias, initFn, gateFn)
	}
}

// locateHandlersGo resolves the absolute path to apiserver/pkg/handlers/handlers.go
// from this test file, independent of the caller's cwd, so both `go test ./...`
// in apiserver/ and `go test ./pkg/mcp/router/...` work the same.
func locateHandlersGo(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// thisFile = .../apiserver/pkg/mcp/router/handlers_wireup_test.go
	// target   = .../apiserver/pkg/handlers/handlers.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "handlers", "handlers.go")
}

// isCallSelector reports whether expr is a call like pkg.fn(...).
func isCallSelector(expr ast.Expr, pkg, fn string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == pkg && sel.Sel.Name == fn
}
