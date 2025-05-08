/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func getWorkspace(ctx context.Context, cli client.Client, workspaceName string) (*v1.Workspace, error) {
	if workspaceName == "" {
		return nil, commonerrors.NewBadRequest("no workspace name provided")
	}
	workspace := &v1.Workspace{}
	if err := cli.Get(ctx, client.ObjectKey{Name: workspaceName}, workspace); err != nil {
		return nil, err
	}
	return workspace, nil
}
