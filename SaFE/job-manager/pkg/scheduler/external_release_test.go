/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/execution"
)

// releaseProvider models the provider's release contract: a request id is bound to the
// first body it carried, expected_revision must match, and reason must be non-empty.
type releaseProvider struct {
	mu            sync.Mutex
	revision      int
	phase         string
	seen          map[string]string
	releases      int
	reuseRefusals int
	emptyReasons  int
	revokingPolls int
}

// claimBody renders the current claim as the provider would return it.
func (p *releaseProvider) claimBody() string {
	body := replacePhase(activeClaimBody, p.phase)
	return replaceOnce(body, `"revision":3`, `"revision":`+strconv.Itoa(p.revision))
}

// ServeHTTP answers release, read and demand withdrawal calls.
func (p *releaseProvider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch {
	case r.Method == http.MethodGet:
		if p.phase == execution.ClaimPhaseRevoking {
			p.revokingPolls++
			if p.revokingPolls > 1 {
				p.phase = execution.ClaimPhaseReleased
			}
		}
		okJSON(p.claimBody())(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/release"):
		p.releases++
		raw, _ := io.ReadAll(r.Body)
		var req execution.ReleaseRequest
		_ = json.Unmarshal(raw, &req)
		if prev, ok := p.seen[req.RequestID]; ok && prev != string(raw) {
			p.reuseRefusals++
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"Conflict","message":"this request_id was already used for a different request","request_id":"r"}`))
			return
		}
		p.seen[req.RequestID] = string(raw)
		if req.Reason == "" {
			p.emptyReasons++
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"InvalidRequest","message":"reason is required","request_id":"r"}`))
			return
		}
		if int(req.ExpectedRevision) != p.revision {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"Conflict","message":"stale revision","request_id":"r"}`))
			return
		}
		if p.phase == execution.ClaimPhaseActive {
			p.phase = execution.ClaimPhaseRevoking
			p.revision++
		}
		okJSON(p.claimBody())(w, r)
	default:
		okJSON(`{"api_version":"safe-exec/v1alpha1","request_id":"r","demand_id":"d1","revision":2,"eligible":false,"reason":"Withdrawn","units":[],"observed_at":"2026-09-29T12:00:00.000Z","expires_at":"2026-09-29T12:05:00.000Z"}`)(w, r)
	}
}

// driveRelease runs reconcileExternalRelease against fresh reads until the hold ends.
func driveRelease(t *testing.T, r *SchedulerReconciler, name string, passes int) bool {
	t.Helper()
	holding := true
	for i := 0; i < passes && holding; i++ {
		stored := &v1.Workload{}
		if err := r.Get(context.Background(), client.ObjectKey{Name: name}, stored); err != nil {
			t.Fatal(err)
		}
		var err error
		if holding, err = r.reconcileExternalRelease(context.Background(), stored); err != nil {
			t.Fatalf("pass %d: %v", i, err)
		}
	}
	return holding
}

// A revision that moved since accept must not leave the release stuck on a reused id.
func TestReleaseAfterStaleRevisionReachesReleased(t *testing.T) {
	p := &releaseProvider{revision: 3, phase: execution.ClaimPhaseActive, seen: map[string]string{}}
	useExecutionServer(t, p)
	w, state := recoveryWorkload()
	w.Status.Phase = v1.WorkloadStopped
	state.ClaimPhase = execution.ClaimPhaseActive
	state.ClaimRevision = 1
	w.Status.ExternalExecution = state
	r, _ := exchangeFixture(t, w)
	if driveRelease(t, r, w.Name, 6) {
		t.Fatal("claim was never released")
	}
	if p.reuseRefusals != 0 {
		t.Fatalf("a request id was reused for a different body %d times", p.reuseRefusals)
	}
}

// Once the provider reports Revoking the release is not sent again; the claim is polled.
func TestRevokingClaimIsPolledNotReleasedAgain(t *testing.T) {
	p := &releaseProvider{revision: 4, phase: execution.ClaimPhaseRevoking, seen: map[string]string{}}
	useExecutionServer(t, p)
	w, state := recoveryWorkload()
	w.Status.Phase = v1.WorkloadStopped
	state.ClaimPhase = execution.ClaimPhaseRevoking
	state.ClaimRevision = 4
	state.Reclaiming = true
	w.Status.ExternalExecution = state
	r, _ := exchangeFixture(t, w)
	if driveRelease(t, r, w.Name, 4) {
		t.Fatal("claim was never released")
	}
	if p.releases != 0 {
		t.Fatalf("release was sent %d times while the claim was Revoking", p.releases)
	}
}

// A workload deleted outside the API has no phase; the release still carries a reason.
func TestReleaseOfDeletedWorkloadCarriesAReason(t *testing.T) {
	p := &releaseProvider{revision: 3, phase: execution.ClaimPhaseActive, seen: map[string]string{}}
	useExecutionServer(t, p)
	w, state := recoveryWorkload()
	w.Status.Phase = ""
	now := metav1.NewTime(time.Now())
	w.DeletionTimestamp = &now
	w.Finalizers = []string{"test/hold"}
	state.ClaimPhase = execution.ClaimPhaseActive
	state.ClaimRevision = 3
	w.Status.ExternalExecution = state
	r, _ := exchangeFixture(t, w)
	if driveRelease(t, r, w.Name, 4) {
		t.Fatal("claim was never released")
	}
	if p.emptyReasons != 0 {
		t.Fatalf("release was sent without a reason %d times", p.emptyReasons)
	}
}
