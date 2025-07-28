/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"os"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func prepareForNodeJob(t *testing.T, jobType string) *Node {
	nsenterSh = ""
	node, _ := newNode(t)
	v1.SetLabel(node.k8sNode, v1.OpsJobTypeLabel, jobType)
	v1.SetLabel(node.k8sNode, v1.OpsJobIdLabel, "test-job")
	if jobType == string(v1.OpsJobAddonType) {
		node.k8sNode.Spec.Taints = []corev1.Taint{{
			Key:    commonfaults.GenerateTaintKey("501"),
			Effect: corev1.TaintEffectNoSchedule,
		}}
	}
	return node
}

func TestAddonJobSucceed(t *testing.T) {
	node := prepareForNodeJob(t, string(v1.OpsJobAddonType))
	nodeJobInput := commonjob.OpsJobInput{
		DispatchTime: time.Now().Unix(),
		Commands: []commonjob.OpsJobCommand{{
			Addon:   "test",
			Action:  stringutil.Base64Encode("touch ./.a;echo hello"),
			Observe: stringutil.Base64Encode("if [ -f \"./.a\" ]; then\n    exit 0\nelse\n    exit 1\nfi"),
		}},
	}
	v1.SetAnnotation(node.k8sNode,
		v1.OpsJobInputAnnotation, string(jsonutils.MarshalSilently(nodeJobInput)))
	defer os.Remove("./.a")

	var job NodeJob
	err := job.reconcile(node)
	time.Sleep(time.Millisecond * 200)

	assert.NilError(t, err)
	cond := node.FindConditionByType(v1.OpsJobKind)
	assert.Equal(t, cond != nil, true)
	assert.Equal(t, v1.GetOpsJobId(node.k8sNode), "test-job")
	assert.Equal(t, cond.Reason, "test-job")
	assert.Equal(t, cond.Status, corev1.ConditionTrue)
	assert.Equal(t, cond.Message, "")
	assert.Equal(t, cond.LastTransitionTime.IsZero(), false)
	assert.Equal(t, cond.LastTransitionTime.Unix() >= nodeJobInput.DispatchTime, true)

	cond = node.FindConditionByType("test")
	assert.Equal(t, cond != nil, true)
	assert.Equal(t, cond.Reason, "test-job")
	assert.Equal(t, cond.Status, corev1.ConditionTrue)
	assert.Equal(t, cond.Message, "hello")
	assert.Equal(t, cond.LastTransitionTime.IsZero(), false)
	assert.Equal(t, cond.LastTransitionTime.Unix() >= nodeJobInput.DispatchTime, true)
}

func TestAddonJobFailed(t *testing.T) {
	node := prepareForNodeJob(t, string(v1.OpsJobAddonType))
	nodeJobInput := commonjob.OpsJobInput{
		DispatchTime: time.Now().Unix(),
		Commands: []commonjob.OpsJobCommand{{
			Addon:  "test",
			Action: stringutil.Base64Encode("echo error\nexit 1"),
		}},
	}
	v1.SetAnnotation(node.k8sNode,
		v1.OpsJobInputAnnotation, string(jsonutils.MarshalSilently(nodeJobInput)))

	var job NodeJob
	err := job.reconcile(node)
	time.Sleep(time.Millisecond * 200)

	assert.Equal(t, err != nil, true)
	cond := node.FindConditionByType(v1.OpsJobKind)
	assert.Equal(t, cond != nil, true)
	assert.Equal(t, v1.GetOpsJobId(node.k8sNode), "test-job")
	assert.Equal(t, cond.Reason, "test-job")
	assert.Equal(t, cond.Status, corev1.ConditionFalse)
	assert.Equal(t, strings.Contains(cond.Message, "error"), true)
	assert.Equal(t, cond.LastTransitionTime.IsZero(), false)
	assert.Equal(t, cond.LastTransitionTime.Unix() >= nodeJobInput.DispatchTime, true)
}
