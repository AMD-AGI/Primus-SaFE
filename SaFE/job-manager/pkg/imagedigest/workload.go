/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package imagedigest

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// ResolveWorkloadImages pins each Spec.Images entry to a digest using pull secrets
// from the control-plane primus-safe namespace (workload SecretEntity image ids).
func ResolveWorkloadImages(ctx context.Context, cli client.Client, workload *v1.Workload) ([]string, error) {
	if workload == nil {
		return nil, fmt.Errorf("nil workload")
	}
	if len(workload.Spec.Images) == 0 {
		return nil, fmt.Errorf("workload %s has no images", workload.Name)
	}
	kc, err := keychainForWorkload(ctx, cli, workload)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(workload.Spec.Images))
	for i, image := range workload.Spec.Images {
		// An empty Spec.Images slot means the role keeps the template default image.
		if image == "" {
			out[i] = ""
			continue
		}
		pinned, resolveErr := ResolveFunc(ctx, image, kc)
		if resolveErr != nil {
			return nil, fmt.Errorf("image %q: %w", image, resolveErr)
		}
		out[i] = pinned
	}
	return out, nil
}

func keychainForWorkload(ctx context.Context, cli client.Client, workload *v1.Workload) (authn.Keychain, error) {
	if cli == nil {
		return authn.DefaultKeychain, nil
	}
	var chains []authn.Keychain
	for _, s := range workload.Spec.Secrets {
		if s.Type != v1.SecretImage || s.Id == "" {
			continue
		}
		secret := &corev1.Secret{}
		err := cli.Get(ctx, client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: s.Id}, secret)
		if err != nil {
			// A declared pull secret that is not synced yet must not fall through to
			// anonymous resolve (which surfaces as a permanent 401/UNAUTHORIZED).
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("image pull secret %q not found: %w", s.Id, err)
			}
			return nil, err
		}
		kc, kcErr := KeychainFromPullSecret(secret)
		if kcErr != nil {
			return nil, kcErr
		}
		chains = append(chains, kc)
	}
	if len(chains) == 0 {
		return authn.DefaultKeychain, nil
	}
	return authn.NewMultiKeychain(append(chains, authn.DefaultKeychain)...), nil
}
