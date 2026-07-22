/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
)

type InformerType int

const (
	DisableInformer       InformerType = 0
	EnableInformer        InformerType = 1
	EnableDynamicInformer InformerType = 2
)

// Health-probe tuning for the built-in connection watchdog. Informer-enabled
// factories actively probe the apiserver so a silently wedged watch (which emits
// no watch error and would otherwise starve callers until a process restart) is
// detected and the factory flipped invalid, letting callers that gate on
// IsValid() stop using — and rebuild — the connection.
const (
	healthProbeInterval = 30 * time.Second
	healthProbeTimeout  = 5 * time.Second
	healthFailThreshold = 3
)

// ClientFactory kubernetes client factory structure for managing cluster connections and Informers
type ClientFactory struct {
	ctx context.Context
	// Factory name, typically refers to cluster name
	name          string
	clientSet     kubernetes.Interface
	restConfig    *rest.Config
	dynamicClient *dynamic.DynamicClient
	// Used by dynamicSharedInformerFactory
	mapper meta.RESTMapper
	// SharedInformerFactory and DynamicSharedInformerFactory do not coexist
	sharedInformerFactory        informers.SharedInformerFactory
	dynamicSharedInformerFactory dynamicinformer.DynamicSharedInformerFactory
	// Used to stop informer factory
	stopCh chan struct{}
	// Informer type enum definition. 0: disable informer; 1: sharedInformer; 2 dynamicSharedInformer
	// default 0
	informerType InformerType
	// validMu guards valid/invalidReason, which are read/written concurrently by
	// the health watchdog and by caller goroutines gating on IsValid().
	validMu sync.RWMutex
	// Whether the ClientFactory is valid
	valid bool
	// If the factory is invalid, explain the reason
	invalidReason string
}

// NewClientFactory creates a new client factory.
func NewClientFactory(ctx context.Context, name, endpoint, certData,
	keyData, caData string, informerType InformerType,
) (*ClientFactory, error) {
	clientSet, restCfg, err := NewClientSet(endpoint, certData, keyData, caData, true)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}
	factory := &ClientFactory{
		ctx:           ctx,
		name:          name,
		clientSet:     clientSet,
		restConfig:    restCfg,
		dynamicClient: dynamicClient,
		informerType:  informerType,
		valid:         true,
	}

	const defaultResyncPeriod = 30 * time.Minute
	switch informerType {
	case EnableInformer:
		factory.stopCh = make(chan struct{})
		factory.sharedInformerFactory = informers.NewSharedInformerFactory(clientSet, defaultResyncPeriod)
	case EnableDynamicInformer:
		factory.stopCh = make(chan struct{})
		httpClient, err := rest.HTTPClientFor(restCfg)
		if err != nil {
			return nil, err
		}
		mapper, err := apiutil.NewDynamicRESTMapper(restCfg, httpClient)
		if err != nil {
			return nil, err
		}
		factory.mapper = mapper
		factory.dynamicSharedInformerFactory = dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, defaultResyncPeriod)
	default:
	}
	// Informer-enabled factories run watches that can silently wedge; start the
	// connection watchdog so every module gets self-healing by default.
	if factory.stopCh != nil {
		factory.runHealthWatchdog()
	}
	klog.Infof("new k8s client factory. name: %s, informer type: %d", name, informerType)
	return factory, nil
}

// NewClientFactoryWithOnlyClient create factory instance with client only (without Informer).
func NewClientFactoryWithOnlyClient(ctx context.Context, name string, clientSet kubernetes.Interface) *ClientFactory {
	return &ClientFactory{
		ctx:       ctx,
		name:      name,
		clientSet: clientSet,
		valid:     true,
	}
}

// Name get factory name.
func (f *ClientFactory) Name() string {
	return f.name
}

// Release factory resources, stop Informer (if enabled).
func (f *ClientFactory) Release() error {
	if f.informerType == EnableInformer || f.informerType == EnableDynamicInformer {
		f.StopInformer()
	}
	return nil
}

// IsValid returns true if factory is valid
func (f *ClientFactory) IsValid() bool {
	f.validMu.RLock()
	defer f.validMu.RUnlock()
	return f.valid
}

// SetValid set factory validity status and reason.
func (f *ClientFactory) SetValid(valid bool, msg string) {
	f.validMu.Lock()
	defer f.validMu.Unlock()
	f.valid = valid
	f.invalidReason = msg
}

// ClientSet get Kubernetes client interface.
func (f *ClientFactory) ClientSet() kubernetes.Interface {
	return f.clientSet
}

// RestConfig get REST config.
func (f *ClientFactory) RestConfig() *rest.Config {
	return f.restConfig
}

// DynamicClient get dynamic client.
func (f *ClientFactory) DynamicClient() *dynamic.DynamicClient {
	return f.dynamicClient
}

// Mapper get REST mapper (for dynamic Informer).
func (f *ClientFactory) Mapper() meta.RESTMapper {
	return f.mapper
}

// SharedInformerFactory get shared Informer factory (only available when standard Informer is enabled).
func (f *ClientFactory) SharedInformerFactory() informers.SharedInformerFactory {
	if f.informerType != EnableInformer {
		return nil
	}
	return f.sharedInformerFactory
}

// DynamicSharedInformerFactory get dynamic shared Informer factory (only available when dynamic Informer is enabled).
func (f *ClientFactory) DynamicSharedInformerFactory() dynamicinformer.DynamicSharedInformerFactory {
	if f.informerType != EnableDynamicInformer {
		return nil
	}
	return f.dynamicSharedInformerFactory
}

// GetInvalidReason get reason for factory invalidity.
func (f *ClientFactory) GetInvalidReason() string {
	f.validMu.RLock()
	defer f.validMu.RUnlock()
	return f.invalidReason
}

// Probe performs an activity-independent liveness check against the apiserver
// using the factory's client (hence the same cached transport the informers
// use), so a wedged watch connection surfaces as a probe failure. An idle
// cluster still answers, so this never false-positives on "no events".
//
// Any received HTTP status (even 4xx/5xx, e.g. an RBAC 403 on /livez) counts as
// a live connection; only a transport-level failure (timeout, refused, EOF,
// TLS) — where no status code is ever set — is reported as an error.
func (f *ClientFactory) Probe(ctx context.Context) error {
	if f.clientSet == nil {
		return fmt.Errorf("client factory %s has no client", f.name)
	}
	res := f.clientSet.Discovery().RESTClient().Get().AbsPath("/livez").Do(ctx)
	var code int
	res.StatusCode(&code)
	if code != 0 {
		return nil
	}
	return res.Error()
}

// runHealthWatchdog periodically probes the apiserver and flips the factory's
// validity, so callers gating on IsValid() stop using — and rebuild — a wedged
// connection. This covers the silent half-open case that per-informer watch
// error handlers never observe. It exits when the factory context is cancelled
// or the informer stop channel is closed (see Release/StopInformer).
func (f *ClientFactory) runHealthWatchdog() {
	go func() {
		ticker := time.NewTicker(healthProbeInterval)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-f.ctx.Done():
				return
			case <-f.stopCh:
				return
			case <-ticker.C:
			}
			probeCtx, cancel := context.WithTimeout(f.ctx, healthProbeTimeout)
			err := f.Probe(probeCtx)
			cancel()
			if err == nil {
				failures = 0
				if !f.IsValid() {
					klog.Infof("client factory %s health probe recovered, marking valid", f.name)
					f.SetValid(true, "")
				}
				continue
			}
			failures++
			klog.ErrorS(err, "client factory health probe failed",
				"cluster", f.name, "consecutiveFailures", failures)
			if failures >= healthFailThreshold && f.IsValid() {
				klog.Warningf("client factory %s marked invalid after %d consecutive probe failures: %v",
					f.name, failures, err)
				f.SetValid(false, fmt.Sprintf("health probe failed: %v", err))
			}
		}
	}()
}

// StartInformer Start Informer factory (start corresponding Informer based on type).
func (f *ClientFactory) StartInformer() {
	switch f.informerType {
	case EnableInformer:
		f.sharedInformerFactory.Start(f.stopCh)
	case EnableDynamicInformer:
		f.dynamicSharedInformerFactory.Start(f.stopCh)
	}
}

// WaitForCacheSync wait for Informer cache sync to complete with optional timeout.
// If timeout is 0 or negative, no timeout is applied and the method will block until sync completes.
func (f *ClientFactory) WaitForCacheSync(timeout time.Duration) bool {
	if timeout <= 0 {
		switch f.informerType {
		case EnableInformer:
			f.sharedInformerFactory.WaitForCacheSync(f.stopCh)
		case EnableDynamicInformer:
			f.dynamicSharedInformerFactory.WaitForCacheSync(f.stopCh)
		}
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})

	go func() {
		switch f.informerType {
		case EnableInformer:
			f.sharedInformerFactory.WaitForCacheSync(f.stopCh)
		case EnableDynamicInformer:
			f.dynamicSharedInformerFactory.WaitForCacheSync(f.stopCh)
		}
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-ctx.Done():
		klog.Warningf("Cache sync timeout for factory %s after %v", f.name, timeout)
	case <-f.stopCh:
		klog.Infof("Cache sync interrupted for factory %s", f.name)
	}
	return false
}

// StopInformer stop Informer factory, close stopCh channel.
func (f *ClientFactory) StopInformer() {
	if f.stopCh != nil && !channel.IsChannelClosed(f.stopCh) {
		close(f.stopCh)
	}
}
