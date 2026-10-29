/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package llmgateway

import (
	"context"
	"errors"
	"time"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"k8s.io/klog/v2"
)

// bindingWaitInterval is how often a request that found another caller creating
// the user's binding looks for the result.
const bindingWaitInterval = 100 * time.Millisecond

// hasApimKey reports whether the binding carries an APIM key. A binding created
// by the proxy for a user who never uploaded one stores the empty string.
func hasApimKey(binding *dbclient.LLMGatewayUserBinding) bool {
	return binding != nil && binding.ApimKey != ""
}

// resolveBinding returns the caller's binding. A user without one gets a
// binding that holds only their LiteLLM virtual key, which LiteLLM needs to
// authenticate and bill the request. The gateway does not know which models sit
// behind APIM: a request without an APIM key is forwarded without one, and
// whatever requires it rejects the call upstream.
func (h *Handler) resolveBinding(ctx context.Context, email string) (*dbclient.LLMGatewayUserBinding, error) {
	binding, err := h.dbClient.GetLLMBindingByEmail(ctx, email)
	if err != nil || binding != nil {
		return binding, err
	}
	return h.ensureBinding(ctx, email)
}

// ensureBinding creates the user's key-less binding exactly once.
//
// Concurrent first requests of one user in this process share a single
// creation (singleflight), and the per-user advisory lock extends that across
// apiserver replicas. The work runs under its own deadline rather than the
// request's, so a caller that disconnects does not fail the callers sharing it.
func (h *Handler) ensureBinding(ctx context.Context, email string) (*dbclient.LLMGatewayUserBinding, error) {
	result := h.bindingFlight.DoChan(email, func() (val any, err error) {
		// DoChan runs this on its own goroutine and turns a panic there into a
		// process crash; as an error it fails the requests like any other.
		defer func() {
			if r := recover(); r != nil {
				klog.ErrorS(nil, "LLM Proxy: panic while creating binding", "email", email, "panic", r)
				err = commonerrors.NewInternalError("internal error")
			}
		}()
		opCtx, cancel := context.WithTimeout(context.Background(), createBindingOperationTimeout)
		defer cancel()
		return h.ensureBindingShared(opCtx, email)
	})
	select {
	case res := <-result:
		if res.Err != nil {
			return nil, res.Err
		}
		// Each caller gets its own copy of the shared result.
		binding := *res.Val.(*dbclient.LLMGatewayUserBinding)
		return &binding, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ensureBindingShared creates the binding under the user's advisory lock. When
// another replica, or the user uploading an APIM key, holds the lock it waits
// for that to finish and uses the binding it produced.
func (h *Handler) ensureBindingShared(ctx context.Context, email string) (*dbclient.LLMGatewayUserBinding, error) {
	for {
		var binding *dbclient.LLMGatewayUserBinding
		err := h.withLLMBindingAdvisoryLock(ctx, email, func(lockedCtx context.Context) error {
			var err error
			binding, err = h.ensureBindingLocked(lockedCtx, email)
			return err
		})
		if err == nil {
			return binding, nil
		}
		if !errors.Is(err, dbclient.ErrLLMBindingLockBusy) {
			return nil, err
		}

		timer := time.NewTimer(bindingWaitInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			klog.ErrorS(ctx.Err(), "LLM Proxy: timed out waiting for binding creation", "email", email)
			return nil, commonerrors.NewInternalError("LLM service unavailable, please try again later")
		case <-timer.C:
		}

		existing, err := h.dbClient.GetLLMBindingByEmail(ctx, email)
		if err != nil {
			klog.ErrorS(err, "LLM Proxy: DB error", "email", email)
			return nil, commonerrors.NewInternalError("internal error")
		}
		if existing != nil {
			return existing, nil
		}
	}
}

// ensureBindingLocked returns the binding, creating a key-less one if it is
// still missing once the lock is held.
func (h *Handler) ensureBindingLocked(ctx context.Context, email string) (*dbclient.LLMGatewayUserBinding, error) {
	existing, err := h.dbClient.GetLLMBindingByEmail(ctx, email)
	if err != nil {
		klog.ErrorS(err, "LLM Proxy: DB error", "email", email)
		return nil, commonerrors.NewInternalError("internal error")
	}
	if existing != nil {
		return existing, nil
	}

	binding, _, err := h.provisionBindingLocked(ctx, email, "")
	if err != nil && commonerrors.IsAlreadyExist(err) {
		// The LiteLLM alias was taken and a binding appeared meanwhile: use it.
		if appeared, getErr := h.dbClient.GetLLMBindingByEmail(ctx, email); getErr == nil && appeared != nil {
			return appeared, nil
		}
	}
	return binding, err
}
