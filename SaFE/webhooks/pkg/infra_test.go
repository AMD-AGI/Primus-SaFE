/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"testing"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// fakeManager is a minimal manager.Manager implementation for webhook registration tests.
type fakeManager struct {
	manager.Manager
	client client.Client
	scheme *runtime.Scheme
}

// GetClient returns the embedded fake client.
func (m *fakeManager) GetClient() client.Client { return m.client }

// GetScheme returns the embedded scheme.
func (m *fakeManager) GetScheme() *runtime.Scheme { return m.scheme }

// TestSetUpWebhooks verifies all webhooks register without error.
func TestSetUpWebhooks(t *testing.T) {
	s := newScheme(t)
	mgr := &fakeManager{
		client: fake.NewClientBuilder().WithScheme(s).Build(),
		scheme: s,
	}
	server := webhook.NewServer(webhook.Options{})
	setUpWebhooks(mgr, server)
}

// TestNewServer verifies server construction fails without required flags.
func TestNewServer(t *testing.T) {
	s, err := NewServer()
	assert.Assert(t, err != nil)
	assert.Assert(t, s == nil)
}

// TestServerInitConfig verifies config initialization error handling.
func TestServerInitConfig(t *testing.T) {
	s := &Server{opts: &Options{Config: "nonexistent-config.yaml"}}
	assert.Assert(t, s.initConfig() != nil)
}

// TestServerNewCtrlManager verifies controller manager creation fails out of cluster.
func TestServerNewCtrlManager(t *testing.T) {
	s := &Server{opts: &Options{}}
	assert.Assert(t, s.newCtrlManager() != nil)
}

// TestServerInitLogs verifies log initialization runs.
func TestServerInitLogs(t *testing.T) {
	s := &Server{opts: &Options{}}
	_ = s.initLogs()
}

// TestServerStop verifies stop runs without panic.
func TestServerStop(t *testing.T) {
	s := &Server{}
	s.Stop()
}

// TestServerStartNotInited verifies start is a no-op when not initialized.
func TestServerStartNotInited(t *testing.T) {
	s := &Server{}
	s.Start()
}
