// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ClusterClient provides methods to interact with the target Kubernetes cluster.
// It wraps both the Go client-go library and kubectl commands for flexibility.
type ClusterClient struct {
	clientset  *kubernetes.Clientset
	kubeconfig []byte
}

// NewClusterClient creates a new ClusterClient from kubeconfig bytes
func NewClusterClient(kubeconfig []byte) (*ClusterClient, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &ClusterClient{
		clientset:  clientset,
		kubeconfig: kubeconfig,
	}, nil
}

// GetKubeconfig returns the kubeconfig bytes
func (c *ClusterClient) GetKubeconfig() []byte {
	return c.kubeconfig
}

// NamespaceExists checks if a namespace exists
func (c *ClusterClient) NamespaceExists(ctx context.Context, name string) (bool, error) {
	_, err := c.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CreateNamespace creates a namespace if it doesn't exist
func (c *ClusterClient) CreateNamespace(ctx context.Context, name string, labels map[string]string) error {
	exists, err := c.NamespaceExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	_, err = c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

// ClusterRoleExists checks if a ClusterRole exists
func (c *ClusterClient) ClusterRoleExists(ctx context.Context, name string) (bool, error) {
	_, err := c.clientset.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeploymentExists checks if a Deployment exists
func (c *ClusterClient) DeploymentExists(ctx context.Context, namespace, name string) (bool, error) {
	_, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeploymentReady checks if a Deployment is ready (all replicas available)
func (c *ClusterClient) DeploymentReady(ctx context.Context, namespace, name string) (bool, error) {
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check if all replicas are ready
	if deploy.Status.ReadyReplicas >= *deploy.Spec.Replicas {
		return true, nil
	}
	return false, nil
}

// WaitForDeploymentReady waits for a deployment to be ready
func (c *ClusterClient) WaitForDeploymentReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ready, err := c.DeploymentReady(ctx, namespace, name)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}

		log.Infof("Waiting for deployment %s/%s to be ready...", namespace, name)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}

	return fmt.Errorf("timeout waiting for deployment %s/%s", namespace, name)
}

// SecretExists checks if a Secret exists
func (c *ClusterClient) SecretExists(ctx context.Context, namespace, name string) (bool, error) {
	_, err := c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetSecret retrieves a Secret
func (c *ClusterClient) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

// WaitForSecret waits for a secret to exist
func (c *ClusterClient) WaitForSecret(ctx context.Context, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		exists, err := c.SecretExists(ctx, namespace, name)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}

		log.Infof("Waiting for secret %s/%s to exist...", namespace, name)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}

	return fmt.Errorf("timeout waiting for secret %s/%s", namespace, name)
}

// StorageClassExists checks if a StorageClass exists
func (c *ClusterClient) StorageClassExists(ctx context.Context, name string) (bool, error) {
	_, err := c.clientset.StorageV1().StorageClasses().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CRDExists checks if a CustomResourceDefinition exists
func (c *ClusterClient) CRDExists(ctx context.Context, name string) (bool, error) {
	// Use kubectl since CRD client requires additional setup
	return c.runKubectlCheck(ctx, "get", "crd", name)
}

// CustomResourceExists checks if a custom resource exists
func (c *ClusterClient) CustomResourceExists(ctx context.Context, apiVersion, kind, namespace, name string) (bool, error) {
	args := []string{"get", kind, name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	return c.runKubectlCheck(ctx, args...)
}

// GetCustomResourceStatus gets the status of a custom resource
func (c *ClusterClient) GetCustomResourceStatus(ctx context.Context, kind, namespace, name, jsonpath string) (string, error) {
	args := []string{"get", kind, name, "-o", fmt.Sprintf("jsonpath=%s", jsonpath)}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	kubeconfigFile, cleanup, err := c.writeKubeconfigTemp()
	if err != nil {
		return "", err
	}
	defer cleanup()

	args = append([]string{"--kubeconfig", kubeconfigFile}, args...)
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl get failed: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ApplyYAML applies a YAML manifest using kubectl
func (c *ClusterClient) ApplyYAML(ctx context.Context, yaml []byte) error {
	kubeconfigFile, cleanup, err := c.writeKubeconfigTemp()
	if err != nil {
		return err
	}
	defer cleanup()

	// Write YAML to temp file
	yamlFile, err := os.CreateTemp("", "manifest-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(yamlFile.Name())

	if _, err := yamlFile.Write(yaml); err != nil {
		return err
	}
	yamlFile.Close()

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigFile, "apply", "-f", yamlFile.Name())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply failed: %s", stderr.String())
	}

	return nil
}

// DeleteResource deletes a resource using kubectl
func (c *ClusterClient) DeleteResource(ctx context.Context, kind, namespace, name string) error {
	kubeconfigFile, cleanup, err := c.writeKubeconfigTemp()
	if err != nil {
		return err
	}
	defer cleanup()

	args := []string{"--kubeconfig", kubeconfigFile, "delete", kind, name, "--ignore-not-found"}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl delete failed: %s", stderr.String())
	}

	return nil
}

// WaitForPods waits for pods matching a label selector to be ready
func (c *ClusterClient) WaitForPods(ctx context.Context, namespace, labelSelector string, timeout time.Duration) error {
	kubeconfigFile, cleanup, err := c.writeKubeconfigTemp()
	if err != nil {
		return err
	}
	defer cleanup()

	args := []string{
		"--kubeconfig", kubeconfigFile,
		"wait", "pods",
		"-n", namespace,
		"-l", labelSelector,
		"--for=condition=Ready",
		"--timeout", timeout.String(),
	}

	log.Infof("Waiting for pods: kubectl %s", strings.Join(args[1:], " "))

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wait for pods failed: %s", stderr.String())
	}

	log.Infof("Pods ready: %s", stdout.String())
	return nil
}

// WaitForPodsWithRetry waits for pods with retry on "no matching resources found"
func (c *ClusterClient) WaitForPodsWithRetry(ctx context.Context, namespace, labelSelector string, timeout time.Duration) error {
	maxRetries := 30
	retryInterval := 20 * time.Second
	startTime := time.Now()

	for retry := 0; retry < maxRetries; retry++ {
		if time.Since(startTime) > timeout {
			return fmt.Errorf("timeout waiting for pods with label %s", labelSelector)
		}

		remainingTimeout := timeout - time.Since(startTime)
		if remainingTimeout < 30*time.Second {
			remainingTimeout = 30 * time.Second
		}

		err := c.WaitForPods(ctx, namespace, labelSelector, remainingTimeout)
		if err == nil {
			return nil
		}

		errStr := err.Error()
		if strings.Contains(errStr, "no matching resources found") {
			log.Infof("Pods with label %s not yet created, waiting... (retry %d/%d)", labelSelector, retry+1, maxRetries)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}

		// Other error
		return err
	}

	return fmt.Errorf("pods with label %s not created after %d retries", labelSelector, maxRetries)
}

// runKubectlCheck runs a kubectl command and returns true if successful
func (c *ClusterClient) runKubectlCheck(ctx context.Context, args ...string) (bool, error) {
	kubeconfigFile, cleanup, err := c.writeKubeconfigTemp()
	if err != nil {
		return false, err
	}
	defer cleanup()

	fullArgs := append([]string{"--kubeconfig", kubeconfigFile}, args...)
	cmd := exec.CommandContext(ctx, "kubectl", fullArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "not found") || strings.Contains(stderr.String(), "NotFound") {
			return false, nil
		}
		return false, fmt.Errorf("kubectl command failed: %s", stderr.String())
	}

	return true, nil
}

// writeKubeconfigTemp writes kubeconfig to a temp file and returns the path and cleanup function
func (c *ClusterClient) writeKubeconfigTemp() (string, func(), error) {
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create kubeconfig temp file: %w", err)
	}

	if _, err := kubeconfigFile.Write(c.kubeconfig); err != nil {
		os.Remove(kubeconfigFile.Name())
		return "", nil, fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	cleanup := func() {
		os.Remove(kubeconfigFile.Name())
	}

	return kubeconfigFile.Name(), cleanup, nil
}
