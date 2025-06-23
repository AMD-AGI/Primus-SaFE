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

	defaultTimeoutSecond = 1800
	maxMessageLen        = 1024
)

func (job *NodeJob) Reconcile(n *Node) error {
	quit, err := job.observe(n)
	if quit || err != nil {
		return err
	}
	jobId := v1.GetOpsJobId(n.k8sNode)
	if err = job.handle(n); err != nil {
		klog.ErrorS(err, "failed to handle node job", "job.id", jobId)
		job.addCondition(n, jobId, err.Error(), corev1.ConditionFalse)
		return err
	}
	// If adding the condition fails, the system will retry.
	if job.addCondition(n, jobId, "", corev1.ConditionTrue) == nil {
		klog.Infof("job(%s) process successfully.", jobId)
	}
	return nil
}

// Observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (job *NodeJob) observe(n *Node) (bool, error) {
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
	// Addon jobs must wait until the required taint is created
	if v1.GetOpsJobType(n.k8sNode) == string(v1.OpsJobAddonType) {
		if len(n.k8sNode.Spec.Taints) == 0 {
			return true, nil
		}
	}
	return false, nil
}

func (job *NodeJob) observeJobProcessed(n *Node) (bool, error) {
	isConditionEqual := func(cond1, cond2 *corev1.NodeCondition) bool {
		if cond1.Type == cond2.Type && cond1.Reason == cond2.Reason {
			return true
		}
		return false
	}
	cond := &corev1.NodeCondition{
		Type:   v1.OpsJobKind,
		Reason: v1.GetOpsJobId(n.k8sNode),
	}
	cond = n.FindCondition(cond, isConditionEqual)
	if cond != nil && !cond.LastTransitionTime.IsZero() {
		if getJobDispatchTime(n.k8sNode) <= cond.LastTransitionTime.Unix() {
			return true, nil
		}
	}
	return false, nil
}

func (job *NodeJob) handle(n *Node) error {
	jobInput := getJobInput(n.k8sNode)
	if jobInput == nil {
		return fmt.Errorf("invalid ops job input")
	}
	var err error
	hasHandled := false
	jobId := v1.GetOpsJobId(n.k8sNode)
	for i, cmd := range jobInput.Commands {
		if !n.IsMatchChip(string(cmd.Chip)) {
			klog.Infof("skipping OpsJob %s: chip %s is not match.", jobId, string(cmd.Chip))
			continue
		}
		if jobInput.Commands[i].IsSystemd {
			if err2 := job.executeSystemd(jobId, cmd); err2 != nil {
				err = err2
			}
		} else {
			if err2 := job.executeCommand(jobId, cmd); err2 != nil {
				err = err2
			}
		}
		hasHandled = true
	}
	if err != nil {
		return err
	}
	if !hasHandled {
		return fmt.Errorf("chip mismatched")
	}
	return nil
}

func (job *NodeJob) executeCommand(jobId string, jobCmd commonjob.OpsJobCommand) error {
	// Verify if the expectation is already satisfied. If yes, return without taking any action.
	if jobCmd.Observe != "" {
		if statusCode, _ := job.execute(jobCmd.Observe); statusCode == types.StatusOk {
			klog.Infof("job(%s) already satisfies expectations", jobId)
			return nil
		}
	}

	if jobCmd.Action != "" {
		if statusCode, output := job.execute(jobCmd.Action); statusCode != types.StatusOk {
			return fmt.Errorf("%s", output)
		}
		klog.Infof("ops job(%s) do action successfully", jobId)
	}

	if jobCmd.Observe == "" {
		return nil
	}
	statusCode, _ := job.execute(jobCmd.Observe)
	switch statusCode {
	case types.StatusOk:
		klog.Infof("ops job(%s) observe successfully", jobId)
		return nil
	case types.StatusError:
		return fmt.Errorf("the observation result does not meet expectation")
	default:
		return fmt.Errorf("failed to do observe")
	}
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
		command = fmt.Sprintf(systemdRestart, serviceName, serviceName)
	} else {
		content = genSystemdService(scriptFullPath)
		if err := utils.WriteFile(serviceFullPath, content, 0644); err != nil {
			return err
		}
		command = fmt.Sprintf(systemdStart, serviceName, serviceName)
	}
	if statusCode, output := job.execute(command); statusCode != types.StatusOk {
		return fmt.Errorf("%s", output)
	}
	klog.Infof("ops job(%s) execute systemd successfully", jobId)
	return nil
}

func (job *NodeJob) execute(cmd string) (int, string) {
	if nsenter != "" {
		cmd = nsenter + "'" + cmd + "'"
	}
	statusCode, output := utils.ExecuteCommand(cmd, time.Second*defaultTimeoutSecond)
	if statusCode != types.StatusOk {
		output = normalizeMessage(output)
	}
	return statusCode, output
}

func (job *NodeJob) addCondition(n *Node, jobId, message string, status corev1.ConditionStatus) error {
	cond := corev1.NodeCondition{
		Type:               v1.OpsJobKind,
		Status:             status,
		Reason:             jobId,
		Message:            message,
		LastTransitionTime: metav1.Time{Time: time.Now().UTC()},
	}
	if err := n.AddConditions(cond); err != nil {
		return err
	}
	return nil
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

func getJobDispatchTime(node *corev1.Node) int64 {
	jobInput := commonjob.GetOpsJobInput(node)
	if jobInput == nil {
		return 0
	}
	return jobInput.DispatchTime
}

func genSystemdService(scriptPath string) string {
	content := "[Unit]\nDescription=Xcs Init\nAfter=network.target\n\n" +
		"[Service]\nExecStart=sudo sh " + scriptPath + "\n\n" +
		"[Install]\nWantedBy=multi-user.target\n"
	return content
}

func normalizeMessage(message string) string {
	if len(message) > maxMessageLen {
		message = message[len(message)-maxMessageLen:]
	}
	message = strings.Replace(message, "\n", " ", -1)
	message = strings.Replace(message, "\t", " ", -1)
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(message, " ")
}
