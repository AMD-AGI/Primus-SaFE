/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsPrimus(t *testing.T) {
	err := NewBadRequest("test")
	assert.Equal(t, IsPrimus(err), true)
	assert.Equal(t, GetErrorCode(err), BadRequest)

	err2 := fmt.Errorf("test")
	assert.Equal(t, IsPrimus(err2), false)
	assert.Equal(t, GetErrorCode(err2), "")
}

func TestIsAlreadyExist(t *testing.T) {
	err := NewAlreadyExist("test")
	assert.Equal(t, IsAlreadyExist(err), true)
	err2 := fmt.Errorf("test")
	assert.Equal(t, IsAlreadyExist(err2), false)

	err3 := apierrors.NewAlreadyExists(schema.GroupResource{}, "test")
	assert.Equal(t, IsAlreadyExist(err3), false)
}
