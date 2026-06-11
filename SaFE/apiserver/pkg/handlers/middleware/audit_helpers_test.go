/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUserFromK8sNoInternalAuth(t *testing.T) {
	// InternalAuth singleton is not initialized in unit tests, so the lookup
	// returns nil rather than panicking.
	assert.Nil(t, getUserFromK8s(context.Background(), "u1"))
}
