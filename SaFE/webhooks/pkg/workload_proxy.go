/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"fmt"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
)

func (m *WorkloadMutator) mutateCICDProxyOptIn(ctx context.Context, oldWorkload, workload *v1.Workload) error {
	if err := validateCICDProxyInput(ctx, m.Client, m.secretReader, workload, oldWorkload); err != nil {
		return err
	}
	if commonworkload.ExpectedCICDProxyOptIn(workload, oldWorkload) {
		v1.SetAnnotation(workload, v1.CICDProxyManagedAnnotation, v1.TrueStr)
	} else {
		v1.RemoveAnnotation(workload, v1.CICDProxyManagedAnnotation)
	}
	return nil
}

func validateCICDProxyAdmission(ctx context.Context, reader, secretReader client.Reader, workload, oldWorkload *v1.Workload) error {
	marker, present := workload.Annotations[v1.CICDProxyManagedAnnotation]
	expected := commonworkload.ExpectedCICDProxyOptIn(workload, oldWorkload)
	if present != expected || (present && marker != v1.TrueStr) {
		return fmt.Errorf("metadata.annotations[%s]: reserved admission marker does not match validated proxy opt-in", v1.CICDProxyManagedAnnotation)
	}
	return validateCICDProxyInput(ctx, reader, secretReader, workload, oldWorkload)
}

func validateCICDProxyInput(ctx context.Context, reader, secretReader client.Reader, workload, oldWorkload *v1.Workload) error {
	if !commonworkload.IsCICD(workload) || (oldWorkload != nil && reflect.DeepEqual(oldWorkload.Spec, workload.Spec)) {
		return nil
	}
	source := workload
	if commonworkload.IsCICDEphemeralRunner(workload) {
		var err error
		source, err = commonworkload.ResolveCICDProxySource(ctx, reader, workload)
		if err != nil {
			return err
		}
		if !commonworkload.IsCICDProxyManaged(source) {
			return nil
		}
		childEnv, err := commonworkload.CICDProxyEnv(workload.Spec.Env)
		if err != nil {
			return err
		}
		parentEnv, err := commonworkload.CICDProxyEnv(source.Spec.Env)
		if err != nil {
			return err
		}
		for _, key := range commonworkload.CICDProxyEnvKeys() {
			if value, exists := childEnv[key]; exists && value != parentEnv[key] {
				return fmt.Errorf("env.%s: must match the owning scale set Workload", key)
			}
		}
	}
	config, err := commonworkload.ParseCICDProxy(source.Spec.Env)
	if err != nil {
		return err
	}
	if secretReader == nil {
		secretReader = reader
	}
	return commonworkload.ReadCICDProxySecret(ctx, secretReader, source, config)
}
