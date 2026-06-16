/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSchemeInitialized(t *testing.T) {
	// The package init must register the API types into the scheme.
	assert.NotNil(t, scheme)
	assert.NotEmpty(t, scheme.AllKnownTypes())
}
