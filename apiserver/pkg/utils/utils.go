/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	DefaultMaxRequestBodyBytes = int64(2 * 1024 * 1024)
)

func ReadBodyToUnstructured(req *http.Request) (unstructured.Unstructured, error) {
	body, err := ReadBody(req)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	// 将 JSON 数据解析为 unstructured.Unstructured 对象
	var unstructuredObj unstructured.Unstructured
	err = json.Unmarshal(body, &unstructuredObj.Object)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	return unstructuredObj, nil
}

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
