/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestResourceMessageKey(t *testing.T) {
	m1 := &resourceMessage{
		cluster:   "c1",
		namespace: "ns",
		name:      "pod-1",
		gvk:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
	}
	// Same object identity -> same key regardless of action/dispatchCount.
	m2 := &resourceMessage{
		cluster:       "c1",
		namespace:     "ns",
		name:          "pod-1",
		gvk:           schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
		action:        ResourceDel,
		dispatchCount: 5,
	}
	assert.Equal(t, resourceMessageKey(m1), resourceMessageKey(m2))

	// Different name -> different key.
	m3 := &resourceMessage{cluster: "c1", namespace: "ns", name: "pod-2",
		gvk: schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}}
	assert.NotEqual(t, resourceMessageKey(m1), resourceMessageKey(m3))
}

func TestMergeResourceMessage(t *testing.T) {
	del := &resourceMessage{name: "o", action: ResourceDel}
	upd := &resourceMessage{name: "o", action: ResourceUpdate}

	// Latest wins when no delete is pending.
	assert.Equal(t, upd, mergeResourceMessage(nil, false, upd))
	assert.Equal(t, del, mergeResourceMessage(upd, true, del))

	// A pending delete is NOT overwritten by a later non-delete.
	assert.Equal(t, del, mergeResourceMessage(del, true, upd))

	// A delete replaces a pending non-delete.
	assert.Equal(t, del, mergeResourceMessage(upd, true, del))
}
