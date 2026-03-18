// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/robust"
)

// RobustFacade wraps a local Facade and can override data-plane sub-facades
// with Robust API-backed implementations. Non-data-plane methods fall through
// to the embedded local Facade.
type RobustFacade struct {
	*database.Facade
	client      *robust.Client
	clusterName string
}

func (f *RobustFacade) WithCluster(clusterName string) database.FacadeInterface {
	client := robust.GetClientForCluster(clusterName)
	if client == nil {
		return f.Facade.WithCluster(clusterName).(*database.Facade)
	}
	localFacade := f.Facade.WithCluster(clusterName).(*database.Facade)
	return &RobustFacade{
		Facade:      localFacade,
		client:      client,
		clusterName: clusterName,
	}
}

func (f *RobustFacade) GetClient() *robust.Client {
	return f.client
}

// initRobustFacadeFactory registers the factory that creates RobustFacade instances.
// Called during server init when data plane mode is not "local".
func initRobustFacadeFactory() {
	database.SetRobustFacadeFactory(func(localFacade *database.Facade, clusterName string) database.FacadeInterface {
		client := robust.GetClientForCluster(clusterName)
		if client == nil {
			return localFacade
		}
		return &RobustFacade{
			Facade:      localFacade,
			client:      client,
			clusterName: clusterName,
		}
	})
}
