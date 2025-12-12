/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exporter

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const (
	// MaxTTLHour defines the maximum time-to-live in hours for deleted objects before they are ignored
	MaxTTLHour = 48
)

// SetupExporters initializes and registers all resource exporters with the controller manager
// It sets up database clients and configures handlers for Workload, Fault, and OpsJob resources
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
					if !obj.GetDeletionTimestamp().IsZero() &&
						time.Now().UTC().Sub(obj.GetDeletionTimestamp().Time).Hours() > MaxTTLHour {
						klog.Errorf("failed to upsert workload(%d), ignore it: %v", dbWorkload.Id, err)
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
				dbFault := faultMapper(obj)
				if dbFault == nil {
					return nil
				}
				dbClient.UpsertFault(ctx, dbFault)
				return nil
			},
			filter: faultFilter,
		},
		{
			gvk: v1.SchemeGroupVersion.WithKind(v1.OpsJobKind),
			handler: func(ctx context.Context, obj *unstructured.Unstructured) error {
				dbJob := opsJobMapper(obj)
				if dbJob == nil {
					return nil
				}
				dbClient.UpsertJob(ctx, dbJob)
				return nil
			},
			filter: nil,
		},
		{
			gvk: v1.SchemeGroupVersion.WithKind(v1.InferenceKind),
			handler: func(ctx context.Context, obj *unstructured.Unstructured) error {
				dbInference := inferenceMapper(obj)
				if dbInference == nil {
					return nil
				}
				if err := dbClient.UpsertInference(ctx, dbInference); err != nil {
					if !obj.GetDeletionTimestamp().IsZero() &&
						time.Now().UTC().Sub(obj.GetDeletionTimestamp().Time).Hours() > MaxTTLHour {
						klog.Errorf("failed to upsert inference(%d), ignore it: %v", dbInference.Id, err)
						return nil
					}
					return err
				}
				return nil
			},
			filter: inferenceFilter,
		},
		{
			gvk: v1.SchemeGroupVersion.WithKind(v1.ModelKind),
			handler: func(ctx context.Context, obj *unstructured.Unstructured) error {
				dbModel := modelMapper(obj)
				if dbModel == nil {
					return nil
				}
				if err := dbClient.UpsertModel(ctx, dbModel); err != nil {
					if !obj.GetDeletionTimestamp().IsZero() &&
						time.Now().UTC().Sub(obj.GetDeletionTimestamp().Time).Hours() > MaxTTLHour {
						klog.Errorf("failed to upsert model(%s), ignore it: %v", dbModel.ID, err)
						return nil
					}
					return err
				}
				return nil
			},
			filter: nil,
		},
	} {
		if err := addExporter(ctx, mgr, toRegister.gvk, toRegister.handler, toRegister.filter); err != nil {
			return err
		}
	}
	return nil
}
