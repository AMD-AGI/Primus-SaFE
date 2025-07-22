/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

type NodeJob struct {
}

const (
	systemdPath    = "/etc/systemd/system"
	systemdStart   = "systemctl enable %s && systemctl start %s"
	systemdRestart = "systemctl reload %s && systemctl restart %s"

	defaultTimeoutSecond = 3600
	maxMessageLen        = 256
)

var (
	nsenterSh = "nsenter --target 1 --mount --uts --ipc --net --pid -- sh -c "
)

func (job *NodeJob) reconcile(n *Node) error {
	quit, err := job.observe(n)
	if quit || err != nil {
		return err
	}
	if err = job.handle(n); err != nil {
		if jobId := v1.GetOpsJobId(n.k8sNode); jobId != "" {
			klog.ErrorS(err, "failed to handle job", "jobid", jobId)
			addJobCondition(n, jobId, err.Error(), corev1.ConditionFalse)
		}
		return err
	}

	// If adding the condition fails, the system will retry.
	if jobId := v1.GetOpsJobId(n.k8sNode); jobId != "" {
		if addJobCondition(n, jobId, "", corev1.ConditionTrue) == nil {
			klog.Infof("job(%s) process successfully.", jobId)
		}
	}
	return nil
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (job *NodeJob) observe(n *Node) (bool, error) {
	if n.k8sNode == nil {
		return false, fmt.Errorf("please initialize node first")
	}

	funcs := []func(*Node) (bool, error){
		job.observeJobInvalidity, job.observeJobProcessed,
	}
	for _, f := range funcs {
		quit, err := f(n)
		if quit || err != nil {
			return true, err
		}
	}
	return false, nil
}

func (job *NodeJob) observeJobInvalidity(n *Node) (bool, error) {
	if v1.GetOpsJobId(n.k8sNode) == "" ||
		v1.GetOpsJobType(n.k8sNode) == "" || v1.GetOpsJobInput(n.k8sNode) == "" {
		return true, nil
	}
	return false, nil
}

func isConditionEqual(cond1, cond2 *corev1.NodeCondition) bool {
	if cond1.Type == cond2.Type && cond1.Reason == cond2.Reason {
		return true
	}
	return false
}

func (job *NodeJob) observeJobProcessed(n *Node) (bool, error) {
	cond := &corev1.NodeCondition{
		Type:   v1.OpsJobKind,
		Reason: v1.GetOpsJobId(n.k8sNode),
	}
	cond = n.FindCondition(cond, isConditionEqual)
	if cond != nil && !cond.LastTransitionTime.IsZero() {
		jobInput := commonjob.GetOpsJobInput(n.k8sNode)
		if jobInput == nil || jobInput.DispatchTime <= cond.LastTransitionTime.Unix() {
			return true, nil
		}
	}
	return false, nil
}

func (job *NodeJob) handle(n *Node) error {
	jobInput := getJobInput(n.k8sNode)
	if jobInput == nil || len(jobInput.Commands) == 0 {
		return fmt.Errorf("invalid input")
	}
	addonConditions := make([]corev1.NodeCondition, 0, len(jobInput.Commands))

	for i, cmd := range jobInput.Commands {
		if !n.IsMatchGpuChip(string(cmd.GpuChip)) {
			continue
		}
		if !n.IsMatchGpuProduct(string(cmd.GpuProduct)) {
			continue
		}
		var err error
		message := ""
		jobId := v1.GetOpsJobId(n.k8sNode)
		if jobInput.Commands[i].IsSystemd {
			err = job.executeSystemd(jobId, cmd)
		} else {
			message, err = job.executeCommand(jobId, cmd)
		}
		// Check again — the task may be gone.
		if v1.GetOpsJobId(n.k8sNode) == "" {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to execute %s, %s", cmd.Addon, err.Error())
		}
		addonConditions = append(addonConditions, corev1.NodeCondition{
			Type:               corev1.NodeConditionType(cmd.Addon),
			Status:             corev1.ConditionTrue,
			Reason:             jobId,
			Message:            message,
			LastTransitionTime: metav1.Time{Time: time.Now().UTC()},
		})
	}
	if len(addonConditions) == 0 {
		return fmt.Errorf("no addon is applicable to the node")
	}
	if err := addAddonConditions(n, addonConditions); err != nil {
		klog.ErrorS(err, "failed to add addon condition", "job", v1.GetOpsJobId(n.k8sNode))
	}
	return nil
}

func (job *NodeJob) executeCommand(jobId string, jobCmd commonjob.OpsJobCommand) (string, error) {
	// Verify if the expectation is already satisfied. If yes, return without taking any action.
	statusCode := 0
	actionMessage := ""
	if jobCmd.Observe != "" {
		if statusCode, _ = job.execute(jobCmd.Observe); statusCode == types.StatusOk {
			klog.Infof("job(%s) already satisfies expectations, addon: %s", jobId, jobCmd.Addon)
			return "", nil
		}
	}

	if jobCmd.Action != "" {
		if statusCode, actionMessage = job.execute(jobCmd.Action); statusCode != types.StatusOk {
			return "", fmt.Errorf("action message: %s, code: %d", actionMessage, statusCode)
		}
		klog.Infof("ops job(%s) execute action successfully, addon: %s", jobId, jobCmd.Addon)
	}

	if jobCmd.Observe == "" {
		return actionMessage, nil
	}
	observeMessage := ""
	statusCode, observeMessage = job.execute(jobCmd.Observe)
	if statusCode != types.StatusOk {
		return "", fmt.Errorf("observe message: %s, code: %d", observeMessage, statusCode)
	}
	klog.Infof("ops job(%s) observe successfully, addon: %s", jobId, jobCmd.Addon)
	return actionMessage, nil
}

func (job *NodeJob) executeSystemd(jobId string, jobCmd commonjob.OpsJobCommand) error {
	content := "#!/bin/bash\n" + jobCmd.Action
	scriptFullPath := fmt.Sprintf("%s/%s.sh", systemdPath, jobCmd.Addon)
	if err := utils.WriteFile(scriptFullPath, content, 0770); err != nil {
		return err
	}

	serviceName := fmt.Sprintf("%s.service", jobCmd.Addon)
	serviceFullPath := fmt.Sprintf("%s/%s", systemdPath, serviceName)
	command := ""
	if utils.IsFileExist(serviceFullPath) {
		if jobCmd.IsOneShotService {
			return nil
		}
		command = fmt.Sprintf(systemdRestart, serviceName, serviceName)
	} else {
		content = genSystemdService(scriptFullPath)
		if err := utils.WriteFile(serviceFullPath, content, 0644); err != nil {
			return err
		}
		command = fmt.Sprintf(systemdStart, serviceName, serviceName)
	}
	if statusCode, output := job.execute(command); statusCode != types.StatusOk {
		return fmt.Errorf("message: %s, code: %d", output, statusCode)
	}
	klog.Infof("ops job(%s) execute systemd successfully, addon: %s", jobId, jobCmd.Addon)
	return nil
}

func (job *NodeJob) execute(cmd string) (int, string) {
	if nsenterSh != "" {
		cmd = nsenterSh + "'" + cmd + "'"
	}
	statusCode, output := utils.ExecuteCommand(cmd, time.Second*defaultTimeoutSecond)
	return statusCode, normalizeMessage(output)
}

func addJobCondition(n *Node, jobId, message string, status corev1.ConditionStatus) error {
	cond := corev1.NodeCondition{
		Type:               v1.OpsJobKind,
		Status:             status,
		Reason:             jobId,
		Message:            message,
		LastTransitionTime: metav1.Time{Time: time.Now().UTC()},
	}

	hasFound := false
	conditions := make([]corev1.NodeCondition, 0, len(n.k8sNode.Status.Conditions)+1)
	for _, currentCond := range n.k8sNode.Status.Conditions {
		if cond.Type == currentCond.Type {
			if cond.Status == currentCond.Status &&
				cond.Message == currentCond.Message && cond.Reason == currentCond.Reason {
				return nil
			}
			hasFound = true
			conditions = append(conditions, cond)
		} else {
			conditions = append(conditions, currentCond)
		}
	}
	if !hasFound {
		conditions = append(conditions, cond)
	}
	return n.UpdateConditions(conditions)
}

func addAddonConditions(n *Node, inputs []corev1.NodeCondition) error {
	conditions := make([]corev1.NodeCondition, 0, len(n.k8sNode.Status.Conditions)+len(inputs))
	conditions = append(conditions, n.k8sNode.Status.Conditions...)
	for _, cond := range inputs {
		if n.FindCondition(&cond, isConditionEqual) != nil {
			continue
		}
		conditions = append(conditions, cond)
	}
	if len(conditions) == len(n.k8sNode.Status.Conditions) {
		return nil
	}
	return n.UpdateConditions(conditions)
}

func getJobInput(node *corev1.Node) *commonjob.OpsJobInput {
	jobInput := commonjob.GetOpsJobInput(node)
	if jobInput == nil || len(jobInput.Commands) == 0 {
		return nil
	}
	for i := range jobInput.Commands {
		jobInput.Commands[i].Action = stringutil.Base64Decode(jobInput.Commands[i].Action)
		jobInput.Commands[i].Observe = stringutil.Base64Decode(jobInput.Commands[i].Observe)
	}
	return jobInput
}

func genSystemdService(scriptPath string) string {
	content := "[Unit]\nDescription=PrimusSafe Init\nAfter=network.target\n\n" +
		"[Service]\nExecStart=sudo sh " + scriptPath + "\n\n" +
		"[Install]\nWantedBy=multi-user.target\n"
	return content
}

func normalizeMessage(message string) string {
	if message == "" {
		return ""
	}
	if len(message) > maxMessageLen {
		message = message[:maxMessageLen]
	}
	message = strings.Replace(message, "\n", " ", -1)
	message = strings.Replace(message, "\t", " ", -1)
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(message, " ")
}
