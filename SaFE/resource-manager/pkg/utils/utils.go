/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

const (
	Username     = "username"
	Password     = "password"
	Root         = "root"
	Authorize    = "authorize"
	AuthorizePub = "authorize.pub"
)

// RemoveOwnerReferences removes owner references with the specified UID from the given references.
func RemoveOwnerReferences(references []metav1.OwnerReference, uid types.UID) []metav1.OwnerReference {
	newReferences := make([]metav1.OwnerReference, 0, len(references))
	for k, r := range references {
		if r.UID != uid {
			newReferences = append(newReferences, references[k])
		}
	}
	return newReferences
}

// RemoveFinalizer removes specified finalizers from the object and updates it.
// Re-fetches the object on ResourceVersion conflict so patches succeed under concurrent status updates.
func RemoveFinalizer(ctx context.Context, cli client.Client, obj client.Object, finalizer ...string) error {
	key := client.ObjectKeyFromObject(obj)
	var lastErr error
	for range 5 {
		cp := obj.DeepCopyObject()
		if cp == nil {
			return fmt.Errorf("RemoveFinalizer: DeepCopyObject returned nil")
		}
		latest, ok := cp.(client.Object)
		if !ok {
			return fmt.Errorf("RemoveFinalizer: unexpected deep copy type %T", cp)
		}
		if err := cli.Get(ctx, key, latest); err != nil {
			return client.IgnoreNotFound(err)
		}
		changed := false
		for _, val := range finalizer {
			if controllerutil.RemoveFinalizer(latest, val) {
				changed = true
			}
		}
		if !changed {
			return nil
		}
		patchErr := commonutils.PatchObjectFinalizer(ctx, cli, latest)
		if patchErr == nil {
			obj.SetFinalizers(latest.GetFinalizers())
			obj.SetResourceVersion(latest.GetResourceVersion())
			return nil
		}
		lastErr = patchErr
		if !apierrors.IsConflict(patchErr) {
			klog.ErrorS(patchErr, "failed to remove finalizer")
			return client.IgnoreNotFound(patchErr)
		}
	}
	klog.ErrorS(lastErr, "failed to remove finalizer after retries")
	return lastErr
}

// IncRetryCount increments the retry count annotation on the object and updates it.
func IncRetryCount(ctx context.Context, cli client.Client, obj client.Object, maxCount int) (int, error) {
	count := v1.GetRetryCount(obj) + 1
	if count > maxCount {
		return count, nil
	}
	originalObj := client.MergeFrom(obj.DeepCopyObject().(client.Object))
	v1.SetAnnotation(obj, v1.RetryCountAnnotation, strconv.Itoa(count))
	if err := cli.Patch(ctx, obj, originalObj); err != nil {
		return 0, client.IgnoreNotFound(err)
	}
	return count, nil
}

// GetK8sClientFactory retrieves the Kubernetes client factory for the specified cluster.
func GetK8sClientFactory(clientManager *commonutils.ObjectManager, clusterId string) (*commonclient.ClientFactory, error) {
	if clientManager == nil {
		return nil, commonerrors.NewInternalError("client manager is empty")
	}
	obj, _ := clientManager.Get(clusterId)
	if obj == nil {
		err := fmt.Errorf("the client for cluster %s is not found. pls retry later", clusterId)
		//	klog.Error(err.Error())
		return nil, commonerrors.NewNotFoundWithMessage(err.Error())
	}
	k8sClients, ok := obj.(*commonclient.ClientFactory)
	if !ok {
		return nil, commonerrors.NewInternalError("invalid client object")
	}
	return k8sClients, nil
}

// GetSSHClient creates an SSH client connection to the specified node.
func GetSSHClient(ctx context.Context, cli client.Client, node *v1.Node) (*ssh.Client, error) {
	config, err := GetSSHConfig(ctx, cli, node)
	if err != nil {
		return nil, err
	}
	if node.Spec.Port == nil {
		return nil, commonerrors.NewInternalError("node port is not specified")
	}
	addr := fmt.Sprintf("%s:%d", node.Spec.PrivateIP, *node.Spec.Port)
	sshClient, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect %s via ssh: %v", addr, err)
	}
	return sshClient, nil
}

// GetSSHConfig creates SSH client configuration from node's SSH secret.
func GetSSHConfig(ctx context.Context, cli client.Client, node *v1.Node) (*ssh.ClientConfig, error) {
	if node.Spec.SSHSecret == nil {
		return nil, commonerrors.NewInternalError("failed to get SSH secret of node")
	}
	secret := new(corev1.Secret)
	if err := cli.Get(ctx, apitypes.NamespacedName{
		Name:      node.Spec.SSHSecret.Name,
		Namespace: node.Spec.SSHSecret.Namespace,
	}, secret); err != nil {
		return nil, err
	}

	var username string
	if data, ok := secret.Data[Username]; ok {
		username = string(data)
	} else {
		username = Root
	}
	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Second * 10,
	}

	if sshPrivateKeyData, ok := secret.Data[Authorize]; ok {
		signer, err := ssh.ParsePrivateKey(sshPrivateKeyData)
		if err != nil {
			return nil, commonerrors.NewInternalError(err.Error())
		}
		sshConfig.Auth = append(sshConfig.Auth, ssh.PublicKeys(signer))
	} else if password, ok := secret.Data[Password]; ok {
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(string(password)))
	} else {
		return nil, commonerrors.NewInternalError("ssh private key or password not found in secret")
	}
	return sshConfig, nil
}
