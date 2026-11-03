/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package plugins links the built-in framework compilers; each registers itself in init(). It is separate from
// package recipe because the plugins import recipe.
package plugins

import (
	_ "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe/frameworks/slime"
)
