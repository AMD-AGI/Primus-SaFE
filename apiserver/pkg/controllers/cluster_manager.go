/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"sync"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

var (
	once           sync.Once
	clusterManager *ClusterManager
)

type ClusterManager struct {
	clusters map[string]*ClusterInfo
	sync.RWMutex
}

func NewClusterManager() *ClusterManager {
	once.Do(func() {
		clusterManager = &ClusterManager{
			clusters: make(map[string]*ClusterInfo),
		}
	})
	return clusterManager
}

func (cm *ClusterManager) Add(name string, info *ClusterInfo) {
	if info == nil || name == "" {
		return
	}
	cm.Lock()
	defer cm.Unlock()
	cm.clusters[name] = info
}

func (cm *ClusterManager) Get(name string) *ClusterInfo {
	cm.RLock()
	defer cm.RUnlock()

	info, ok := cm.clusters[name]
	if !ok {
		return nil
	}
	return info
}

func (cm *ClusterManager) Delete(name string) {
	cm.Lock()
	defer cm.Unlock()

	_, ok := cm.clusters[name]
	if !ok {
		return
	}
	delete(cm.clusters, name)
}

type ClusterInfo struct {
	ControlPlane  v1.ControlPlaneStatus
	ClientSet     kubernetes.Interface
	DynamicClient dynamic.Interface
}
