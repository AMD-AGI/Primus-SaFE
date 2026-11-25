/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// withReadOnlyStorage returns a workload that passes full create validation, with
// v1.WorkspaceStorageReadOnlyAnnotation set to val, or without it when val is "".
func withReadOnlyStorage(val string) *v1.Workload {
	w := fullValidWorkload()
	v1.SetLabel(w, v1.UserIdLabel, "alice")
	if val != "" {
		v1.SetAnnotation(w, v1.WorkspaceStorageReadOnlyAnnotation, val)
	}
	return w
}

// The mutating webhook keeps the annotation as the request set it, and still defaults
// use.workspace.storage to true beside it.
func TestWorkspaceStorageReadOnlyKeptByMutator(t *testing.T) {
	m := &WorkloadMutator{Client: fullWorkloadEnvClient(t)}
	w := withReadOnlyStorage(v1.TrueStr)
	m.mutateMeta(context.Background(), w, nil)
	assert.Equal(t, v1.GetAnnotation(w, v1.WorkspaceStorageReadOnlyAnnotation), v1.TrueStr)
	assert.Assert(t, v1.IsWorkspaceStorageReadOnly(w))
	assert.Assert(t, v1.IsEnableWorkspaceStorage(w))

	w = withReadOnlyStorage("")
	m.mutateMeta(context.Background(), w, nil)
	assert.Assert(t, !v1.HasAnnotation(w, v1.WorkspaceStorageReadOnlyAnnotation))
	assert.Assert(t, !v1.IsWorkspaceStorageReadOnly(w))
}

// A create is admitted with the annotation unset, "true" or "false", and refused with any other
// value.
func TestWorkspaceStorageReadOnlyValues(t *testing.T) {
	v := &WorkloadValidator{Client: fullWorkloadEnvClient(t), decoder: newDecoder(t)}
	create := func(w *v1.Workload) (bool, string) {
		resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, w, nil))
		msg := ""
		if resp.Result != nil {
			msg = resp.Result.Message
		}
		return resp.Allowed, msg
	}
	for _, val := range []string{"", v1.TrueStr, "false"} {
		allowed, msg := create(withReadOnlyStorage(val))
		assert.Assert(t, allowed, "%q was refused: %s", val, msg)
	}
	for _, val := range []string{"yes", "True", "readOnly"} {
		allowed, msg := create(withReadOnlyStorage(val))
		assert.Assert(t, !allowed, "%q was admitted", val)
		assert.Assert(t, strings.Contains(msg, v1.WorkspaceStorageReadOnlyAnnotation), "%q: %s", val, msg)
	}
}

// An update cannot turn read-only workspace storage on or off: off would give a running workload
// write access it was created without. Setting "false" where it was unset changes nothing and is
// admitted.
func TestWorkspaceStorageReadOnlyImmutable(t *testing.T) {
	v := &WorkloadValidator{Client: fullWorkloadEnvClient(t), decoder: newDecoder(t)}
	update := func(newW, oldW *v1.Workload) (bool, string) {
		resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, newW, oldW))
		msg := ""
		if resp.Result != nil {
			msg = resp.Result.Message
		}
		return resp.Allowed, msg
	}
	allowed, msg := update(withReadOnlyStorage(v1.TrueStr), withReadOnlyStorage(v1.TrueStr))
	assert.Assert(t, allowed, "an update that keeps the annotation was refused: %s", msg)
	allowed, msg = update(withReadOnlyStorage("false"), withReadOnlyStorage(""))
	assert.Assert(t, allowed, "an update to \"false\" from unset was refused: %s", msg)

	for name, c := range map[string]struct{ from, to string }{
		"removed":    {v1.TrueStr, ""},
		"turned off": {v1.TrueStr, "false"},
		"turned on":  {"", v1.TrueStr},
	} {
		allowed, msg = update(withReadOnlyStorage(c.to), withReadOnlyStorage(c.from))
		assert.Assert(t, !allowed, "%s: admitted", name)
		assert.Assert(t, strings.Contains(msg, v1.WorkspaceStorageReadOnlyAnnotation) &&
			strings.Contains(msg, "immutable"), "%s: refused for another reason: %s", name, msg)
	}
}

// A github runner keeps its state in a writable workspace volume, so it cannot have read-only
// workspace storage.
func TestWorkspaceStorageReadOnlyRefusedForGithubRunner(t *testing.T) {
	w := githubRunnerForLabelTest("runner-a", "pool-a")
	assert.NilError(t, validateWorkspaceStorageReadOnly(w))
	v1.SetAnnotation(w, v1.WorkspaceStorageReadOnlyAnnotation, v1.TrueStr)
	assert.ErrorContains(t, validateWorkspaceStorageReadOnly(w), "writable workspace storage")
	v1.SetAnnotation(w, v1.WorkspaceStorageReadOnlyAnnotation, "false")
	assert.NilError(t, validateWorkspaceStorageReadOnly(w))
}
