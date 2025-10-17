/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package k8sclient

import (
	"context"

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

// ClientFactory - Kubernetes client factory structure for managing cluster connections and Informers
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
	// Default is 0
	informerType InformerType
	// Whether the ClientFactory is valid
	valid bool
	// If the factory is invalid, explain the reason
	invalidReason string
}

// NewClientFactory(ctx context.Context, name, endpoint, certData, keyData, caData string, informerType InformerType) (*ClientFactory, error)
// Create new client factory instance
// Parameters:
//
//	ctx: Context
//	name: Factory name, typically refers to cluster name
//	endpoint: Kubernetes API server endpoint
//	certData: Base64 encoded client certificate data
//	keyData: Base64 encoded client key data
//	caData: Base64 encoded CA certificate data
//	informerType: Informer type enum definition. 0: disable informer; 1: sharedInformer; 2 dynamicSharedInformer
//
// Returns:
//
//	*ClientFactory: Client factory instance
//	error: Error information
func NewClientFactory(ctx context.Context, name, endpoint, certData,
	keyData, caData string, informerType InformerType) (*ClientFactory, error) {
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

	switch informerType {
	case EnableInformer:
		factory.stopCh = make(chan struct{})
		factory.sharedInformerFactory = informers.NewSharedInformerFactory(clientSet, 0)
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
		factory.dynamicSharedInformerFactory = dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 0)
	default:
	}
	klog.Infof("new k8s client factory. name: %s, informer type: %d", name, informerType)
	return factory, nil
}

// Create factory instance with client only (without Informer)
func NewClientFactoryWithOnlyClient(ctx context.Context, name string, clientSet kubernetes.Interface) *ClientFactory {
	return &ClientFactory{
		ctx:       ctx,
		name:      name,
		clientSet: clientSet,
		valid:     true,
	}
}

// Get factory name
// Returns: Factory name string
func (f *ClientFactory) Name() string {
	return f.name
}

// Release factory resources, stop Informer (if enabled)
// Returns: Error information
func (f *ClientFactory) Release() error {
	if f.informerType == EnableInformer || f.informerType == EnableDynamicInformer {
		f.StopInformer()
	}
	return nil
}

// Check if factory is valid
// Returns: true if valid, false if invalid
func (f *ClientFactory) IsValid() bool {
	return f.valid
}

// Set factory validity status and reason
// Parameters:
//
//	valid: Validity status
//	msg: Invalid reason information
func (f *ClientFactory) SetValid(valid bool, msg string) {
	f.valid = valid
	f.invalidReason = msg
}

// Get Kubernetes client interface
// Returns: kubernetes.Interface client interface
func (f *ClientFactory) ClientSet() kubernetes.Interface {
	return f.clientSet
}

// Get REST config
func (f *ClientFactory) RestConfig() *rest.Config {
	return f.restConfig
}

// Get dynamic client
func (f *ClientFactory) DynamicClient() *dynamic.DynamicClient {
	return f.dynamicClient
}

// Get REST mapper (for dynamic Informer)
func (f *ClientFactory) Mapper() meta.RESTMapper {
	return f.mapper
}

// Get shared Informer factory (only available when standard Informer is enabled)
// Returns: informers.SharedInformerFactory
func (f *ClientFactory) SharedInformerFactory() informers.SharedInformerFactory {
	if f.informerType != EnableInformer {
		return nil
	}
	return f.sharedInformerFactory
}

// Get dynamic shared Informer factory (only available when dynamic Informer is enabled)
// Returns: dynamicinformer.DynamicSharedInformerFactory
func (f *ClientFactory) DynamicSharedInformerFactory() dynamicinformer.DynamicSharedInformerFactory {
	if f.informerType != EnableDynamicInformer {
		return nil
	}
	return f.dynamicSharedInformerFactory
}

// Get reason for factory invalidity
func (f *ClientFactory) GetInvalidReason() string {
	return f.invalidReason
}

// Start Informer factory (start corresponding Informer based on type)
func (f *ClientFactory) StartInformer() {
	switch f.informerType {
	case EnableInformer:
		f.sharedInformerFactory.Start(f.stopCh)
	case EnableDynamicInformer:
		f.dynamicSharedInformerFactory.Start(f.stopCh)
	}
}

// Wait for Informer cache sync to complete
func (f *ClientFactory) WaitForCacheSync() {
	switch f.informerType {
	case EnableInformer:
		f.sharedInformerFactory.WaitForCacheSync(f.stopCh)
	case EnableDynamicInformer:
		f.dynamicSharedInformerFactory.WaitForCacheSync(f.stopCh)
	}
}

// Stop Informer factory, close stopCh channel
func (f *ClientFactory) StopInformer() {
	if f.stopCh != nil && !channel.IsChannelClosed(f.stopCh) {
		close(f.stopCh)
	}
}
