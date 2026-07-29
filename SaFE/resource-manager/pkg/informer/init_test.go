/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package informer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestInitInformerProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cl := ctrlfake.NewClientBuilder().Build()
	err := InitInformer(ctx, &rest.Config{Host: "http://127.0.0.1:60999"}, cl)
	assert.NoError(t, err)
}
