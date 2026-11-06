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

// The creator's id on a Workload is what its external Pods carry as primus-safe.user.id, and
// the provider mounts a user directory only into a Pod labelled with that user. An update that
// changes the id, sets one where there was none, or removes it is refused at the validating
// webhook, so an update cannot give a workload another user's directory on its next dispatch.
// An update that leaves the id alone is admitted, so the refusal is the id and nothing else.
func TestWorkloadUpdateCannotChangeTheCreatorUserId(t *testing.T) {
	withUser := func(userId string) *v1.Workload {
		w := fullValidWorkload()
		if userId != "" {
			v1.SetLabel(w, v1.UserIdLabel, userId)
		}
		return w
	}
	v := &WorkloadValidator{Client: fullWorkloadEnvClient(t), decoder: newDecoder(t)}
	handle := func(newW, oldW *v1.Workload) (bool, string) {
		resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, newW, oldW))
		msg := ""
		if resp.Result != nil {
			msg = resp.Result.Message
		}
		return resp.Allowed, msg
	}

	allowed, msg := handle(withUser("alice"), withUser("alice"))
	assert.Assert(t, allowed, "an update that keeps the creator was refused: %s", msg)

	unrelated := withUser("alice")
	v1.SetLabel(unrelated, "team", "x")
	allowed, msg = handle(unrelated, withUser("alice"))
	assert.Assert(t, allowed, "an update of another label was refused: %s", msg)

	for name, c := range map[string]struct{ from, to string }{
		"changed": {"alice", "mallory"},
		"set":     {"", "mallory"},
		"removed": {"alice", ""},
	} {
		allowed, msg = handle(withUser(c.to), withUser(c.from))
		assert.Assert(t, !allowed, "%s: an update of the creator from %q to %q was admitted", name, c.from, c.to)
		assert.Assert(t, strings.Contains(msg, v1.UserIdLabel) && strings.Contains(msg, "immutable"),
			"%s: refused for another reason: %s", name, msg)
	}
}
