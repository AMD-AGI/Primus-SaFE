/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"bytes"
	"context"
	"fmt"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type RebootJobReconciler struct {
	*OpsJobBaseReconciler
}

// SetupRebootJobController initializes and registers the RebootJobReconciler with the controller manager.
func SetupRebootJobController(mgr manager.Manager) error {
	r := &RebootJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{}, onFirstPhaseChangedPredicate()))).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Reboot Job Controller successfully")
	return nil
}

// Reconcile is the main control loop for RebootJob resources.
func (r *RebootJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	var clearFuncs []ClearFunc
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r, clearFuncs...)
}

// observe the job status. Returns true if the expected state is met (no handling required), false otherwise.
func (r *RebootJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	if job.IsEnd() {
		return true, nil
	}

	_, phase := r.getTheUnprocessedNodes(job)
	switch phase {
	case v1.OpsJobPending, "":
		return false, nil
	case v1.OpsJobRunning:
		return false, nil
	case v1.OpsJobFailed, v1.OpsJobSucceeded:
		if err := r.setJobCompleted(ctx, job, phase, "", nil); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// filter determines if the job should be processed by this reconciler.
func (r *RebootJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobRebootType
}

// handle  processes the job based on its current phase and updates its status accordingly.
func (r *RebootJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	nodes, _ := r.getTheUnprocessedNodes(job)
	for _, nodeId := range nodes {
		if err := r.execReboot(ctx, job.Name, nodeId); err != nil {
			klog.Errorf("failed to execute reboot job %s node %s: %v", job.Name, nodeId, err)
		}
	}

	return ctrlruntime.Result{}, nil
}

// execReboot executes the reboot command on the specified node.
func (r *RebootJobReconciler) execReboot(ctx context.Context, jobId, nodeId string) error {
	node := &v1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: nodeId}, node); err != nil {
		return err
	}

	sshClient, err := utils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		return fmt.Errorf("failed to get ssh client: %w, node: %s", err, node.Name)
	}
	defer sshClient.Close()

	cmd := "sudo reboot"
	_, _ = r.executeSSHCommand(sshClient, cmd)
	klog.Infof("machine node %s reboot", node.Name)

	if err = r.setJobOutput(ctx, jobId, nodeId); err != nil {
		return err
	}

	return nil
}

// setJobOutput sets the job output for the specified node.
func (r *RebootJobReconciler) setJobOutput(ctx context.Context, jobId, nodeId string) error {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobId}, job); err != nil {
		return err
	}
	job.Status.Outputs = append(job.Status.Outputs, v1.Parameter{
		Name:  v1.ParameterNode,
		Value: nodeId,
	})

	return r.Status().Update(ctx, job)
}

// executeSSHCommand executes a command via SSH on the specified node.
func (r *RebootJobReconciler) executeSSHCommand(sshClient *ssh.Client, command string) (string, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	session.Stdout = &b
	defer session.Close()

	if err = session.Run(command); err != nil {
		return "", fmt.Errorf("failed to execute command '%s': %v", command, err)
	}
	return b.String(), nil
}

// getTheUnprocessedNodes returns the nodes that have not been processed by the job.
func (r *RebootJobReconciler) getTheUnprocessedNodes(job *v1.OpsJob) ([]string, v1.OpsJobPhase) {
	outputMap := make(map[string]string)
	for _, output := range job.Status.Outputs {
		outputMap[output.Name] = output.Value
	}
	var (
		nodes []string
	)
	for _, input := range job.Spec.Inputs {
		if input.Name != v1.ParameterNode {
			continue
		}
		if _, ok := outputMap[input.Value]; !ok {
			nodes = append(nodes, input.Value)
		}

	}

	if len(nodes) == 0 {
		return nil, v1.OpsJobSucceeded
	} else if job.Status.Phase == v1.OpsJobPending {
		return nil, v1.OpsJobPending
	} else {
		return nodes, v1.OpsJobRunning
	}
}
