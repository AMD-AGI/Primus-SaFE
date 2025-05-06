/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsAlreadyExist(t *testing.T) {
	err := NewAlreadyExist("test")
	assert.Equal(t, IsAlreadyExist(err), true)
	err2 := fmt.Errorf("test")
	assert.Equal(t, IsAlreadyExist(err2), false)
	err3 := NewInternalError("test")
	assert.Equal(t, IsAlreadyExist(err3), false)
	err4 := apierrors.NewAlreadyExists(schema.GroupResource{}, "test")
	assert.Equal(t, IsAlreadyExist(err4), false)
}
