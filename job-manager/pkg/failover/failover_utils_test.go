/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package failover

import (
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestAddConfig(t *testing.T) {
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.PrimusFailover,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string]string{
			"key11": `{"id": "key1"}`,
			"key22": `{"id": "key2"}`,
		},
	}
	configManager := commonutils.NewObjectManager()
	addFailoverConfig(cm1, configManager)

	ok := isMonitorIdExists(configManager, "key1")
	assert.Equal(t, ok, true)
	ok = isMonitorIdExists(configManager, "key2")
	assert.Equal(t, ok, true)
	ok = isMonitorIdExists(configManager, "key3")
	assert.Equal(t, ok, false)

	cm1 = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.PrimusFailover,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string]string{
			"key11": `{"id": "key1"}`,
			"key33": `{"id": "key3"}`,
		},
	}
	addFailoverConfig(cm1, configManager)
	ok = isMonitorIdExists(configManager, "key1")
	assert.Equal(t, ok, true)
	ok = isMonitorIdExists(configManager, "key2")
	assert.Equal(t, ok, false)
	ok = isMonitorIdExists(configManager, "key3")
	assert.Equal(t, ok, true)
}
