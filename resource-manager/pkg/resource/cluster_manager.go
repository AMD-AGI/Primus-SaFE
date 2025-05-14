/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"fmt"
	"sync"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
)

type ClusterInformer struct {
	name      string
	cert      v1.ControlPlaneStatus
	clientSet kubernetes.Interface
	informers.SharedInformerFactory
	stopCh        chan struct{}
	valid         bool
	invalidReason string
}

func newClusterInformer(name string, controlPlane *v1.ControlPlaneStatus) (*ClusterInformer, error) {
	serviceUrl := fmt.Sprintf("https://%s.%s.svc", name, common.PrimusSafeNamespace)
	clientSet, _, err := commonclient.NewClientSet(serviceUrl,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, true)
	if err != nil {
		return nil, err
	}
	informer := &ClusterInformer{
		name:                  name,
		cert:                  *controlPlane,
		clientSet:             clientSet,
		SharedInformerFactory: informers.NewSharedInformerFactory(clientSet, 0),
		stopCh:                make(chan struct{}),
		valid:                 true,
	}
	klog.Infof("new cluster informer. name: %s, service: %s", name, serviceUrl)
	return informer, nil
}

func (ci *ClusterInformer) IsValid() bool {
	return ci.valid
}

func (ci *ClusterInformer) SetValid(valid bool, msg string) {
	ci.valid = valid
	ci.invalidReason = msg
}

func (ci *ClusterInformer) GetClient() kubernetes.Interface {
	return ci.clientSet
}

func (ci *ClusterInformer) GetInvalidReason() string {
	return ci.invalidReason
}

func (ci *ClusterInformer) Start() {
	go ci.SharedInformerFactory.Start(ci.stopCh)
}

func (ci *ClusterInformer) WaitCache() {
	ci.SharedInformerFactory.WaitForCacheSync(ci.stopCh)
}

func (ci *ClusterInformer) Stop() {
	if ci.stopCh != nil && !channel.IsChannelClosed(ci.stopCh) {
		close(ci.stopCh)
	}
}

var (
	once           sync.Once
	clusterManager *ClusterManager
)

func newClusterManager() *ClusterManager {
	once.Do(func() {
		clusterManager = &ClusterManager{
			Informers: make(map[string]*ClusterInformer),
		}
	})
	return clusterManager
}

type ClusterManager struct {
	Informers map[string]*ClusterInformer
	sync.RWMutex
}

func (cm *ClusterManager) getInformer(name string) *ClusterInformer {
	cm.RLock()
	defer cm.RUnlock()

	informer, ok := cm.Informers[name]
	if !ok {
		return nil
	}
	return informer
}

func (cm *ClusterManager) deleteInformer(name string) {
	cm.Lock()
	defer cm.Unlock()

	informer, ok := cm.Informers[name]
	if !ok {
		return
	}
	if informer != nil {
		informer.Stop()
	}
	delete(cm.Informers, name)
}
