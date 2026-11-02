/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package authtest holds fixtures for tests of code that sits behind authentication. It is imported by
// tests only.
package authtest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// chartRoles is the file that defines the built-in roles, as the chart ships them.
const chartRoles = "charts/primus-safe-cr/templates/user/role.yaml"

// ChartRoles returns the built-in roles exactly as the chart installs them (system-admin,
// system-admin-readonly, default, workspace-admin, self-read), so that a test of authorization
// runs against what a deployment has and not against a copy that agrees with the code under test.
// It fails the test when the file cannot be read: a test that silently runs without the roles
// would pass for the wrong reason.
func ChartRoles(t testing.TB) []client.Object {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the chart that defines the built-in roles")
	}
	// here is .../SaFE/apiserver/pkg/handlers/authority/authtest/roles.go
	root := filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, chartRoles))
	if err != nil {
		t.Fatalf("reading the built-in roles: %v", err)
	}
	text := string(raw)
	// The only template actions in the file are the release annotations.
	text = strings.NewReplacer("{{ .Release.Name }}", "release", "{{ .Release.Namespace }}", "namespace").Replace(text)
	var roles []client.Object
	for _, doc := range strings.Split(text, "\n---\n") {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		role := &v1.Role{}
		if err := yaml.Unmarshal([]byte(doc), role); err != nil {
			t.Fatalf("parsing a built-in role: %v\n%s", err, doc)
		}
		if role.Name == "" {
			t.Fatalf("a built-in role has no name: %s", doc)
		}
		roles = append(roles, role)
	}
	return roles
}

// ChartRole returns one built-in role by name; it fails the test when the chart does not define it.
func ChartRole(t testing.TB, name v1.UserRole) *v1.Role {
	t.Helper()
	for _, o := range ChartRoles(t) {
		if r := o.(*v1.Role); r.Name == string(name) {
			return r
		}
	}
	t.Fatalf("the chart defines no %s role", name)
	return nil
}
