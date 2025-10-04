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
			Name:      FailoverConfigmapName,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string]string{
			"key11": `{"key": "key1", "action": "global_restart"}`,
			"key22": `{"key": "key2", "action": "global_restart"}`,
		},
	}
	configManager := commonutils.NewObjectManager()
	addFailoverConfig(cm1, configManager)

	config := getFailoverConfig(configManager, "key1")
	assert.Equal(t, config != nil, true)
	assert.Equal(t, config.Action, "global_restart")
	config = getFailoverConfig(configManager, "key2")
	assert.Equal(t, config != nil, true)
	config = getFailoverConfig(configManager, "key3")
	assert.Equal(t, config == nil, true)

	cm1 = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      FailoverConfigmapName,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string]string{
			"key11": `{"key": "key1", "action": "global_restart"}`,
			"key33": `{"key": "key3", "action": "global_restart"}`,
		},
	}
	addFailoverConfig(cm1, configManager)
	config = getFailoverConfig(configManager, "key1")
	assert.Equal(t, config != nil, true)
	config = getFailoverConfig(configManager, "key2")
	assert.Equal(t, config == nil, true)
	config = getFailoverConfig(configManager, "key3")
	assert.Equal(t, config != nil, true)
}
