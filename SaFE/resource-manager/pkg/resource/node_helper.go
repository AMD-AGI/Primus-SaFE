/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

// dataPlaneNodeLister returns the data plane's node lister, but only once that informer is
// actually running and its cache has filled.
//
// The distinction matters because a lister that has not synced answers NotFound for every
// node in the cluster, and a caller that reads NotFound as "the node is gone" would take that
// for an empty cluster. Nothing guarantees the ordering either: the informer is attached by
// NodeK8sReconciler when its cluster comes up, so a workspace reconcile can perfectly well
// run first. Handing back nil until HasSynced is what keeps a cold cache from answering.
//
// Asking the factory for the Nodes informer registers one as a side effect on clusters whose
// NodeK8sReconciler has not attached yet. That is inert: a registered informer does not run
// until StartInformer, which only that attach path calls, and the attach is handed this same
// shared instance -- with the event handlers and watch error handler it adds to it.
func dataPlaneNodeLister(k8sClients *commonclient.ClientFactory) corev1listers.NodeLister {
	factory := k8sClients.SharedInformerFactory()
	if factory == nil {
		return nil
	}
	nodes := factory.Core().V1().Nodes()
	if !nodes.Informer().HasSynced() {
		return nil
	}
	return nodes.Lister()
}

// getDataPlaneNode reads a node straight from the data plane's apiserver.
//
// This is the read for callers that cannot accept a stale answer: ones that write the object
// back -- a full Update, or a patch carrying the resourceVersion, either of which starts
// failing on a cached copy as soon as kubelet's next heartbeat lands -- and ones that treat
// NotFound as a permanent verdict rather than as something to look at again next round.
// Where a lagging miss only costs a round, use getCachedDataPlaneNode.
func getDataPlaneNode(ctx context.Context, k8sClients *commonclient.ClientFactory, nodeName string) (*corev1.Node, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("the node name is empty")
	}
	result, err := k8sClients.ClientSet().CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return result.DeepCopy(), nil
}

// getCachedDataPlaneNode reads the shared informer's cache, and falls back to the apiserver
// when that cache is not live yet.
//
// It exists for the paths that were paying a serial apiserver round trip per node -- once per
// candidate in the scale-up scan, once per admin Node event -- on the goroutine holding the
// work queue item, while the informer NodeK8sReconciler already runs was holding exactly
// those objects.
//
// The price is that a node the cache has not caught up on reads as NotFound. Only take it
// where that costs a round and no more: the scale-up scan picks the node up on its next
// reconcile, and the node-event path is re-driven by the very events that fill the cache. It
// is the wrong read wherever absence is acted on as a verdict -- isK8sNodePresent refuses a
// binding on it, permanently, and clears the request that asked for it.
func getCachedDataPlaneNode(ctx context.Context, k8sClients *commonclient.ClientFactory, nodeName string) (*corev1.Node, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("the node name is empty")
	}
	if lister := dataPlaneNodeLister(k8sClients); lister != nil {
		// The lister hands out pointers into the shared cache; callers here mutate or store
		// what they get back, so the copy is not optional.
		result, err := lister.Get(nodeName)
		if err != nil {
			return nil, err
		}
		return result.DeepCopy(), nil
	}
	return getDataPlaneNode(ctx, k8sClients, nodeName)
}

// isNeedAuthorization If the SSH secret of the cluster is the same as that of the node, no authorization is required.
// Otherwise, cluster-level authorization is needed, and the cluster's secret should be returned.
func isNeedAuthorization(ctx context.Context, cli client.Client, node *v1.Node, cluster *v1.Cluster) (bool, *corev1.Secret, error) {
	var err error
	secret := new(corev1.Secret)
	if cluster.Spec.ControlPlane.SSHSecret == nil {
		err = cli.Get(ctx, apitypes.NamespacedName{
			Namespace: common.PrimusSafeNamespace,
			Name:      cluster.Name,
		}, secret)
	} else if node.Spec.SSHSecret == nil ||
		node.Spec.SSHSecret.Namespace != cluster.Spec.ControlPlane.SSHSecret.Namespace ||
		node.Spec.SSHSecret.Name != cluster.Spec.ControlPlane.SSHSecret.Name {
		err = cli.Get(ctx, apitypes.NamespacedName{
			Namespace: cluster.Spec.ControlPlane.SSHSecret.Namespace,
			Name:      cluster.Spec.ControlPlane.SSHSecret.Name,
		}, secret)
	} else {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("failed to get cluster secret %s. err: %v", cluster.Name, err)
	}
	return true, secret, nil
}

// isAlreadyAuthorized checks if the cluster's public key exists on the node.
// Returns true if authorized (key exists), false otherwise.
func isAlreadyAuthorized(username string, secret *corev1.Secret, sshClient *ssh.Client) (bool, error) {
	session, err := sshClient.NewSession()
	if err != nil {
		return false, err
	}
	var b bytes.Buffer
	session.Stdout = &b

	var cmd string
	if username == "" || username == "root" {
		cmd = "sudo cat /root/.ssh/authorized_keys"
	} else {
		cmd = fmt.Sprintf("sudo cat /home/%s/.ssh/authorized_keys", username)
	}
	if err = session.Run(cmd); err != nil {
		klog.Errorf("failed exec %s : %v", cmd, err)
	} else {
		pub := string(secret.Data[utils.AuthorizePub])
		index := strings.Index(strings.Replace(b.String(), "\n", "", -1), strings.Replace(pub, "\n", "", -1))
		if index != -1 {
			return true, nil
		}
	}
	return false, nil
}

// getKubeSprayScaleUpCMD generates the command for scaling up a Kubernetes cluster node.
func getKubeSprayScaleUpCMD(user, node, env string) string {
	return fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s scale.yml --limit=%s %s -e \"{'download_run_once': false, 'download_localhost': false}\" -e download_parallel=5 --become-user=root -b -vvv", utils.Authorize, node, env)
}

// getKubeSprayScaleDownCMD generates the command for scaling down a Kubernetes cluster node.
func getKubeSprayScaleDownCMD(user, node, env string) string {
	return fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s remove-node.yml -e node=%s -e skip_confirmation=yes -e reset_nodes=true -e allow_ungraceful_removal=false %s --become-user=root -b -vvv", utils.Authorize, node, env)
}

// getHostname retrieves the hostname of the remote machine via SSH.
func getHostname(conn *ssh.Client) (string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	session.Stdout = &b
	if err = session.Run("hostname"); err != nil {
		return "", fmt.Errorf("failed get hostname: %v", err)
	}
	return strings.Replace(b.String(), "\n", "", -1), nil
}

// setHostname sets the hostname of the remote machine via SSH.
func setHostname(conn *ssh.Client, hostname string) error {
	session, err := conn.NewSession()
	if err != nil {
		return err
	}
	var b bytes.Buffer
	session.Stdout = &b
	if err = session.Run(fmt.Sprintf("sudo hostnamectl set-hostname %s", hostname)); err != nil {
		return fmt.Errorf("failed set hostname: %v", err)
	}
	return nil
}

// isCommandSuccessful checks if a command with the given name has succeeded.
func isCommandSuccessful(status []v1.CommandStatus, name string) bool {
	for _, v := range status {
		if v.Name == name && v.Phase == v1.CommandSucceeded {
			return true
		}
	}
	return false
}

// setCommandStatus updates or adds a command status to the command status list.
func setCommandStatus(commandStatus []v1.CommandStatus, name string, phase v1.CommandPhase) []v1.CommandStatus {
	for k, v := range commandStatus {
		if v.Name == name {
			commandStatus[k].Phase = phase
			return commandStatus
		}
	}
	commandStatus = append(commandStatus, v1.CommandStatus{
		Name:  name,
		Phase: phase,
	})
	return commandStatus
}

// isK8sNodeReady checks if a Kubernetes node is in ready state.
func isK8sNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			return false
		}
	}
	return true
}

// isControlPlaneNode determines if a node is a control plane node.
func isControlPlaneNode(node *v1.Node) bool {
	if v1.IsControlPlane(node) {
		return true
	}
	return false
}

// isConditionsChanged checks if node conditions have changed between old and new conditions.
func isConditionsChanged(oldConditions, newConditions []corev1.NodeCondition) bool {
	if len(oldConditions) != len(newConditions) {
		return true
	}
	oldCondMap := make(map[corev1.NodeConditionType]corev1.NodeCondition, len(oldConditions))
	for i := range oldConditions {
		oldCondMap[oldConditions[i].Type] = oldConditions[i]
	}
	for _, newCond := range newConditions {
		oldCond, ok := oldCondMap[newCond.Type]
		if !ok || oldCond.Status != newCond.Status ||
			oldCond.Reason != newCond.Reason || oldCond.Message != newCond.Message {
			return true
		}
	}
	return false
}

// genNodeOwnerReference generates an owner reference for a node.
func genNodeOwnerReference(node *v1.Node) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         node.APIVersion,
		Kind:               node.Kind,
		Name:               node.Name,
		UID:                node.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}
}
