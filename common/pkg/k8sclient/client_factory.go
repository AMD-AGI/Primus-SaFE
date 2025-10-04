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

type ClientFactory struct {
	ctx context.Context
	// the name of factory. It typically refers to the cluster name
	name          string
	clientSet     kubernetes.Interface
	restConfig    *rest.Config
	dynamicClient *dynamic.DynamicClient
	// it is used by dynamicSharedInformerFactory
	mapper meta.RESTMapper
	// sharedInformerFactory and dynamicSharedInformerFactory do not coexist
	sharedInformerFactory        informers.SharedInformerFactory
	dynamicSharedInformerFactory dynamicinformer.DynamicSharedInformerFactory
	// it is used to stop informer factory
	stopCh chan struct{}
	// default is 0, which disables the informer
	informerType  InformerType
	valid         bool
	invalidReason string
}

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

func NewClientFactoryWithOnlyClient(ctx context.Context, name string, clientSet kubernetes.Interface) *ClientFactory {
	return &ClientFactory{
		ctx:       ctx,
		name:      name,
		clientSet: clientSet,
		valid:     true,
	}
}

func (f *ClientFactory) Name() string {
	return f.name
}

func (f *ClientFactory) Release() error {
	if f.informerType == EnableInformer || f.informerType == EnableDynamicInformer {
		f.StopInformer()
	}
	return nil
}

func (f *ClientFactory) IsValid() bool {
	return f.valid
}

func (f *ClientFactory) SetValid(valid bool, msg string) {
	f.valid = valid
	f.invalidReason = msg
}

func (f *ClientFactory) ClientSet() kubernetes.Interface {
	return f.clientSet
}

func (f *ClientFactory) RestConfig() *rest.Config {
	return f.restConfig
}

func (f *ClientFactory) DynamicClient() *dynamic.DynamicClient {
	return f.dynamicClient
}

func (f *ClientFactory) Mapper() meta.RESTMapper {
	return f.mapper
}

func (f *ClientFactory) SharedInformerFactory() informers.SharedInformerFactory {
	if f.informerType != EnableInformer {
		return nil
	}
	return f.sharedInformerFactory
}

func (f *ClientFactory) DynamicSharedInformerFactory() dynamicinformer.DynamicSharedInformerFactory {
	if f.informerType != EnableDynamicInformer {
		return nil
	}
	return f.dynamicSharedInformerFactory
}

func (f *ClientFactory) GetInvalidReason() string {
	return f.invalidReason
}

func (f *ClientFactory) StartInformer() {
	switch f.informerType {
	case EnableInformer:
		f.sharedInformerFactory.Start(f.stopCh)
	case EnableDynamicInformer:
		f.dynamicSharedInformerFactory.Start(f.stopCh)
	}
}

func (f *ClientFactory) WaitForCacheSync() {
	switch f.informerType {
	case EnableInformer:
		f.sharedInformerFactory.WaitForCacheSync(f.stopCh)
	case EnableDynamicInformer:
		f.dynamicSharedInformerFactory.WaitForCacheSync(f.stopCh)
	}
}

func (f *ClientFactory) StopInformer() {
	if f.stopCh != nil && !channel.IsChannelClosed(f.stopCh) {
		close(f.stopCh)
	}
}
