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
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	DefaultMaxRequestBodyBytes = int64(2 * 1024 * 1024)
)

// ReadBody reads the HTTP request body with a size limit to prevent excessive memory consumption.
// It uses a LimitedReader to restrict the maximum number of bytes that can be read.
// Returns the request body data as bytes, or an error if reading fails or the body exceeds the size limit.
// The request body is automatically closed after reading.
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

// ParseRequestBody reads the request body and unmarshals it into the provided struct.
// It returns the raw body bytes and any error encountered during the process.
// If the body is empty, it returns nil for both body and error.
// If JSON unmarshaling fails, it returns a BadRequest error with the unmarshaling error details.
func ParseRequestBody(req *http.Request, bodyStruct interface{}) ([]byte, error) {
	body, err := ReadBody(req)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	if err = jsonutils.Unmarshal(body, bodyStruct); err != nil {
		return body, commonerrors.NewBadRequest(err.Error())
	}
	return body, nil
}

// GetK8sClientFactory retrieves a Kubernetes client factory for the specified cluster from the client manager.
// Returns the client factory if found and valid, or an error if others
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
