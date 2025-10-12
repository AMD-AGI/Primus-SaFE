/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"fmt"
	"io"
	"net/http"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	DefaultMaxRequestBodyBytes = int64(2 * 1024 * 1024)
)

// Read the HTTP request body with a size limit.
func ReadBody(req *http.Request) ([]byte, error) {
	defer req.Body.Close()
	var lr *io.LimitedReader
	data, err := func() ([]byte, error) {
		lr = &io.LimitedReader{
			R: req.Body,
			N: DefaultMaxRequestBodyBytes + 1,
		}
		return io.ReadAll(lr)
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
		err := fmt.Errorf("the client of cluster %s is not found. please retry later", clusterId)
		return nil, commonerrors.NewInternalError(err.Error())
	}
	k8sClients, ok := obj.(*commonclient.ClientFactory)
	if !ok {
		return nil, commonerrors.NewInternalError("the object type is not matched")
	}
	return k8sClients, nil
}
