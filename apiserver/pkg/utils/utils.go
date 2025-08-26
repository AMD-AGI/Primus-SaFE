/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	DefaultMaxRequestBodyBytes = int64(2 * 1024 * 1024)
)

func ReadBody(req *http.Request) ([]byte, error) {
	defer req.Body.Close()
	var lr *io.LimitedReader
	data, err := func() ([]byte, error) {
		lr = &io.LimitedReader{
			R: req.Body,
			N: DefaultMaxRequestBodyBytes + 1,
		}
		return ioutil.ReadAll(lr)
	}()
	if err != nil {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	if lr != nil && lr.N <= 0 {
		return nil, commonerrors.NewRequestEntityTooLargeError(
			fmt.Sprintf("the max length is %d bytes", DefaultMaxRequestBodyBytes))
	}
	return data, nil
}

func GetK8sClientFactory(clientManager *commonutils.ObjectManager, clusterId string) (*commonclient.ClientFactory, error) {
	obj, _ := clientManager.Get(clusterId)
	if obj == nil {
		err := fmt.Errorf("the client of cluster %s is not found. pls retry later", clusterId)
		return nil, commonerrors.NewInternalError(err.Error())
	}
	k8sClients, ok := obj.(*commonclient.ClientFactory)
	if !ok {
		return nil, commonerrors.NewInternalError("the object type is not matched")
	}
	return k8sClients, nil
}

func GetRoles(ctx context.Context, cli client.Client, user *v1.User) []*v1.Role {
	if user == nil {
		return nil
	}
	var result []*v1.Role
	for _, r := range user.Spec.Roles {
		role := &v1.Role{}
		err := cli.Get(ctx, client.ObjectKey{Name: string(r)}, role)
		if err != nil {
			klog.ErrorS(err, "failed to get user role", "user", user.Name, "role", r)
			continue
		}
		result = append(result, role)
	}
	return result
}
