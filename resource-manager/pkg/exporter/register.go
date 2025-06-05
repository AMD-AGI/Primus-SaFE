/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
)

func SetupExporters(ctx context.Context, mgr manager.Manager) error {
	if !commonconfig.IsDBEnable() {
		return nil
	}
	dbClient := dbclient.NewClient()
	if dbClient == nil {
		return fmt.Errorf("failed to new db client")
	}

	for _, toRegister := range []struct {
		gvk     schema.GroupVersionKind
		handler ResourceHandler
		filter  ResourceFilter
	}{
		{
			gvk: v1.SchemeGroupVersion.WithKind(v1.WorkloadKind),
			handler: func(ctx context.Context, obj *unstructured.Unstructured) error {
				dbWorkload := workloadMapper(obj)
				if dbWorkload == nil {
					return nil
				}
				if err := dbClient.UpsertWorkload(ctx, dbWorkload); err != nil {
					delTime := dbutils.ParseNullTime(dbWorkload.DeleteTime)
					if !delTime.IsZero() && time.Now().UTC().Sub(delTime).Hours() > 72 {
						return nil
					}
					return err
				}
				return nil
			},
			filter: workloadFilter,
		},
		{
			gvk: v1.SchemeGroupVersion.WithKind(v1.FaultKind),
			handler: func(ctx context.Context, obj *unstructured.Unstructured) error {
				dbClient.UpsertFault(ctx, faultMapper(obj))
				return nil
			},
			filter: faultFilter,
		},
	} {
		if err := addExporter(ctx, mgr, toRegister.gvk, toRegister.handler, toRegister.filter); err != nil {
			return err
		}
	}
	return nil
}
