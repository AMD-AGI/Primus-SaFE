/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/secure"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
)

const (
	// DefaultHttpServiePort is the default port for HTTPS service
	DefaultHttpServiePort = 443
	// DefaultApiserverPort is the default port for Kubernetes API server
	DefaultApiserverPort = 6443
)

// guaranteeClusterControlPlane ensures the cluster control plane is in the desired state.
// It handles both creation and deletion of the control plane components.
func (r *ClusterReconciler) guaranteeClusterControlPlane(ctx context.Context, cluster *v1.Cluster) error {
	klog.Infof("cluster %s, phase %s", cluster.Name, cluster.Status.ControlPlaneStatus.Phase)

	if len(cluster.Spec.ControlPlane.Nodes) == 0 {
		return nil
	}

	if err := r.patchKubeControlPlanNodes(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to patch control plane nodes", "cluster", cluster.Name)
		return err
	}

	if err := r.fetchProvisionedClusterKubeConfig(ctx, cluster); err != nil {
		klog.ErrorS(err, "failed to fetch cluster kubeconfig", "cluster", cluster.Name)
		return err
	}

	if guaranteeControllerPlane(cluster) {
		return r.handleControlPlaneCreation(ctx, cluster)
	}

	return r.clearPods(ctx, cluster)
}

// handleControlPlaneCreation manages the creation workflow for control plane
func (r *ClusterReconciler) handleControlPlaneCreation(ctx context.Context, cluster *v1.Cluster) error {
	hostsContent, err := r.generateHosts(ctx, cluster, nil)
	if err != nil {
		klog.ErrorS(err, "failed to generate hosts content", "cluster", cluster.Name)
		if !cluster.DeletionTimestamp.IsZero() {
			return r.removeFinalizer(ctx, cluster)
		}
		return err
	}

	if cluster.Status.ControlPlaneStatus.Phase != v1.ReadyPhase && hostsContent == nil && cluster.DeletionTimestamp.IsZero() {
		klog.Infof("cluster %s Kube control plane nodes not ready, please wait", cluster.Name)
		return nil
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reset(ctx, cluster, hostsContent)
	}

	if err = r.addFinalizer(ctx, cluster); err != nil {
		return err
	}

	return r.createControlPlanePod(ctx, cluster, hostsContent)
}

// removeFinalizer removes the cluster finalizer during deletion
func (r *ClusterReconciler) removeFinalizer(ctx context.Context, cluster *v1.Cluster) error {
	klog.Infof("delete %s finalizer", v1.ClusterFinalizer)
	var ok bool
	cluster.Finalizers, ok = slice.RemoveString(cluster.Finalizers, v1.ClusterFinalizer)
	if !ok {
		return nil
	}
	return r.Update(ctx, cluster)
}

// createControlPlanePod creates and manages the worker pod for cluster creation
func (r *ClusterReconciler) createControlPlanePod(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) error {
	if cluster.Spec.ControlPlane.SSHSecret == nil {
		if err := r.generateSSHSecret(ctx, cluster); err != nil {
			klog.ErrorS(err, "failed to generate ssh secret", "cluster", cluster.Name)
			return err
		}
	}

	pod, err := r.guaranteeCreateWorkerPodCreated(ctx, cluster, hostsContent)
	if err != nil {
		klog.ErrorS(err, "failed to create worker pod for cluster creation", "cluster", cluster.Name)
		return err
	}

	if pod == nil {
		return nil
	}

	return r.updatePodStatus(ctx, cluster, pod)
}

// updatePodStatus updates cluster phase based on pod status
func (r *ClusterReconciler) updatePodStatus(ctx context.Context, cluster *v1.Cluster, pod *corev1.Pod) error {
	originalCluster := client.MergeFrom(cluster.DeepCopy())
	cluster.Status.ControlPlaneStatus.Phase = v1.CreatingPhase

	if pod.Status.Phase == corev1.PodSucceeded {
		cluster.Status.ControlPlaneStatus.Phase = v1.CreatedPhase
	} else if pod.Status.Phase == corev1.PodFailed {
		cluster.Status.ControlPlaneStatus.Phase = v1.CreationFailed
	}

	return r.Status().Patch(ctx, cluster, originalCluster)
}

// reset handles the reset process for a cluster's control plane.
// It manages the deletion phase and reset worker pod creation.
func (r *ClusterReconciler) reset(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) error {
	originalCluster := client.MergeFrom(cluster.DeepCopy())

	if hostsContent == nil {
		cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
		return r.Status().Patch(ctx, cluster, originalCluster)
	}

	if cluster.Status.ControlPlaneStatus.Phase == v1.DeletedPhase {
		return r.patchKubeControlPlanNodes(ctx, cluster)
	}

	if cluster.Status.ControlPlaneStatus.Phase == v1.CreationFailed || cluster.Status.ControlPlaneStatus.Phase == v1.PendingPhase {
		cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
		return r.Status().Patch(ctx, cluster, originalCluster)
	}

	return r.handleResetPodCreation(ctx, cluster, originalCluster, hostsContent)
}

// handleResetPodCreation creates reset worker pod and updates cluster status
func (r *ClusterReconciler) handleResetPodCreation(ctx context.Context, cluster *v1.Cluster, originalCluster client.Patch, hostsContent *HostTemplateContent) error {
	pod, err := r.guaranteeResetWorkPodCreated(ctx, cluster, hostsContent)
	if err != nil {
		return err
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         common.DefaultVersion,
		Kind:               common.PodKind,
		Name:               pod.Name,
		UID:                pod.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}

	if _, err = r.guaranteeHostsConfigMapCreated(ctx, cluster.Name, ownerRef, hostsContent); err != nil {
		return err
	}

	r.updateResetPhase(cluster, pod)
	return r.Status().Patch(ctx, cluster, originalCluster)
}

// updateResetPhase updates cluster phase based on reset pod status
func (r *ClusterReconciler) updateResetPhase(cluster *v1.Cluster, pod *corev1.Pod) {
	if pod.Status.Phase == corev1.PodSucceeded {
		cluster.Status.ControlPlaneStatus.Phase = v1.DeletedPhase
	} else if pod.Status.Phase == corev1.PodFailed {
		cluster.Status.ControlPlaneStatus.Phase = v1.DeleteFailedPhase
	} else {
		cluster.Status.ControlPlaneStatus.Phase = v1.DeletingPhase
	}
}

// getUsername retrieves the username for SSH access to the cluster's control plane node
func (r *ClusterReconciler) getUsername(ctx context.Context, cluster *v1.Cluster) (string, error) {
	if len(cluster.Spec.ControlPlane.Nodes) == 0 {
		return "", fmt.Errorf("no control plane node specified")
	}

	node := &v1.Node{}
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ControlPlane.Nodes[0]}, node)
	if err != nil {
		return "", err
	}

	return r.ClusterBaseReconciler.getUsername(ctx, node, cluster)
}

// guaranteeCreateWorkerPodCreated ensures the worker pod for cluster creation is created.
// It checks for existing pods and creates a new one if needed.
func (r *ClusterReconciler) guaranteeCreateWorkerPodCreated(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	podName := fmt.Sprintf("%s-%s", cluster.Name, v1.ClusterCreateAction)
	pod := new(corev1.Pod)
	err := r.Get(ctx, types.NamespacedName{Namespace: common.PrimusSafeNamespace, Name: podName}, pod)

	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	if err == nil {
		return r.handleExistingPod(ctx, cluster, pod, hostsContent)
	}

	return r.createNewWorkerPod(ctx, cluster, hostsContent)
}

// handleExistingPod processes an existing pod and creates hosts configmap if valid
func (r *ClusterReconciler) handleExistingPod(ctx context.Context, cluster *v1.Cluster, pod *corev1.Pod, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == cluster.Kind && owner.UID == cluster.UID {
			ownerRef := metav1.OwnerReference{
				APIVersion:         common.DefaultVersion,
				Kind:               common.PodKind,
				Name:               pod.Name,
				UID:                pod.UID,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}
			_, err := r.guaranteeHostsConfigMapCreated(ctx, cluster.Name, ownerRef, hostsContent)
			return pod, err
		}
	}

	return nil, r.Delete(ctx, pod)
}

// createNewWorkerPod creates a new worker pod for cluster creation
func (r *ClusterReconciler) createNewWorkerPod(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	username, err := r.getUsername(ctx, cluster)
	if err != nil {
		return nil, err
	}

	cmd := getKubeSprayCreateCMD(username, getKubeSprayEnv(cluster))
	pod := generateWorkerPod(v1.ClusterCreateAction, cluster, username, cmd, getKubesprayImage(cluster), cluster.Name, hostsContent)

	r.addOwnerReferences(pod, hostsContent)

	if err = r.Create(ctx, pod); err != nil {
		return nil, err
	}

	ownerRef := metav1.OwnerReference{
		APIVersion:         common.DefaultVersion,
		Kind:               common.PodKind,
		Name:               pod.Name,
		UID:                pod.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}

	if _, err = r.guaranteeHostsConfigMapCreated(ctx, cluster.Name, ownerRef, hostsContent); err != nil {
		return nil, err
	}

	return pod, nil
}

// addOwnerReferences adds owner references to the pod from hosts content
func (r *ClusterReconciler) addOwnerReferences(pod *corev1.Pod, hostsContent *HostTemplateContent) {
	for _, m := range hostsContent.Controllers {
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			APIVersion: m.APIVersion,
			Kind:       m.Kind,
			Name:       m.Name,
			UID:        m.UID,
		})
	}
}

// guaranteeResetWorkPodCreated ensures the worker pod for cluster reset is created.
// It checks for existing reset pods and creates a new one if needed.
func (r *ClusterReconciler) guaranteeResetWorkPodCreated(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	labelSelector := client.MatchingLabels{
		v1.ClusterManageActionLabel:  string(v1.ClusterResetAction),
		v1.ClusterManageClusterLabel: cluster.Name,
	}

	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}

	if len(list.Items) > 0 {
		return &list.Items[0], nil
	}

	return r.createResetPod(ctx, cluster, hostsContent)
}

// createResetPod creates a new reset worker pod
func (r *ClusterReconciler) createResetPod(ctx context.Context, cluster *v1.Cluster, hostsContent *HostTemplateContent) (*corev1.Pod, error) {
	username, err := r.getUsername(ctx, cluster)
	if err != nil {
		return nil, err
	}

	cmd := getKubeSprayResetCMD(username, getKubeSprayEnv(cluster))
	pod := generateWorkerPod(v1.ClusterResetAction, cluster, username, cmd, getKubesprayImage(cluster), cluster.Name, hostsContent)

	if err = r.Create(ctx, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

// getControllerPlaneNodes retrieves all control plane nodes for the cluster
func (r *ClusterReconciler) getControllerPlaneNodes(ctx context.Context, cluster *v1.Cluster) ([]*v1.Node, error) {
	nodes := make([]*v1.Node, 0, len(cluster.Spec.ControlPlane.Nodes))

	for _, v := range cluster.Spec.ControlPlane.Nodes {
		node := new(v1.Node)
		if err := r.Get(ctx, types.NamespacedName{Name: v}, node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node.DeepCopy())
	}

	return nodes, nil
}

// fetchProvisionedClusterKubeConfig fetches and stores the kubeconfig for a provisioned cluster.
// It connects via SSH to retrieve the config and validates it.
func (r *ClusterReconciler) fetchProvisionedClusterKubeConfig(ctx context.Context, cluster *v1.Cluster) error {
	if !r.shouldFetchKubeConfig(cluster) {
		return nil
	}

	nodes, err := r.getControllerPlaneNodes(ctx, cluster)
	if err != nil || len(nodes) == 0 {
		return err
	}

	config, err := r.fetchConfigFromSSH(ctx, nodes[0])
	if err != nil {
		return err
	}

	return r.updateClusterKubeConfig(ctx, cluster, nodes, config)
}

// shouldFetchKubeConfig determines if kubeconfig should be fetched
func (r *ClusterReconciler) shouldFetchKubeConfig(cluster *v1.Cluster) bool {
	if cluster.Status.ControlPlaneStatus.Phase != v1.CreatedPhase && cluster.Status.ControlPlaneStatus.Phase != "" {
		return false
	}

	hasData := len(cluster.Status.ControlPlaneStatus.CAData) != 0 &&
		len(cluster.Status.ControlPlaneStatus.CertData) != 0 &&
		len(cluster.Status.ControlPlaneStatus.KeyData) != 0

	return !hasData
}

// fetchConfigFromSSH retrieves and validates kubeconfig via SSH
func (r *ClusterReconciler) fetchConfigFromSSH(ctx context.Context, node *v1.Node) (*rest.Config, error) {
	sshClient, err := utils.GetSSHClient(ctx, r.Client, node)
	if err != nil {
		return nil, err
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session failed %s", err)
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b

	if err = session.Run(fmt.Sprintf("sudo cat %s", ProvisionedKubeConfigPath)); err != nil {
		klog.ErrorS(err, "failed to get config", "path", ProvisionedKubeConfigPath)
		return nil, nil
	}

	config, err := clientcmd.Load(b.Bytes())
	if err != nil {
		klog.ErrorS(err, "failed to load config")
		return nil, nil
	}

	conf, err := clientcmd.NewNonInteractiveClientConfig(*config, "", &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("parse config failed %+v", err)
	}

	conf.Host = fmt.Sprintf("https://%s:%d", node.Spec.PrivateIP, DefaultApiserverPort)
	return conf, err
}

// updateClusterKubeConfig updates cluster status with kubeconfig data
func (r *ClusterReconciler) updateClusterKubeConfig(ctx context.Context, cluster *v1.Cluster, nodes []*v1.Node, restConfig *rest.Config) error {
	if restConfig == nil {
		return nil
	}

	cli, err := k8sclient.NewClientSetWithRestConfig(restConfig)
	if err != nil {
		klog.ErrorS(err, "failed to newForConfig", "cluster", cluster.Name)
		return nil
	}

	if _, err = cli.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err != nil {
		klog.ErrorS(err, "failed to list node", "cluster", cluster.Name)
		return nil
	}

	originalCluster := client.MergeFrom(cluster.DeepCopy())

	cluster.Status.ControlPlaneStatus.CertData = base64.StdEncoding.EncodeToString(restConfig.CertData)
	cluster.Status.ControlPlaneStatus.CAData = base64.StdEncoding.EncodeToString(restConfig.CAData)
	cluster.Status.ControlPlaneStatus.KeyData = base64.StdEncoding.EncodeToString(restConfig.KeyData)

	cluster.Status.ControlPlaneStatus.Endpoints = make([]string, 0, len(nodes))
	for _, n := range nodes {
		endpoint := fmt.Sprintf("https://%s:%d", n.Spec.PrivateIP, DefaultApiserverPort)
		cluster.Status.ControlPlaneStatus.Endpoints = append(cluster.Status.ControlPlaneStatus.Endpoints, endpoint)
	}

	cluster.Status.ControlPlaneStatus.Phase = v1.ReadyPhase

	if err = r.guaranteeService(ctx, cluster); err != nil {
		return err
	}

	if err = r.Status().Patch(ctx, cluster, originalCluster); err != nil {
		return fmt.Errorf("failed load config %+v", err)
	}

	return nil
}

// addFinalizer adds the cluster finalizer to the cluster resource if not already present
func (r *ClusterReconciler) addFinalizer(ctx context.Context, cluster *v1.Cluster) error {
	if slice.Contains(cluster.Finalizers, v1.ClusterFinalizer) {
		return nil
	}

	cluster.Finalizers = append(cluster.Finalizers, v1.ClusterFinalizer)
	err := r.Update(ctx, cluster)
	if err != nil {
		return fmt.Errorf("add cluster finalizer failed %+v", err)
	}

	return nil
}

// patchKubeControlPlanNodes updates the control plane nodes with cluster ownership information
func (r *ClusterReconciler) patchKubeControlPlanNodes(ctx context.Context, cluster *v1.Cluster) error {
	patch := func(ctx context.Context, name string) error {
		node, err := r.getNode(ctx, name)
		if err != nil {
			klog.Errorf("patch machine node failed %+v", err)
			if errors.IsNotFound(err) {
				return nil
			}
			return err
		}
		return r.patchMachineNode(ctx, cluster, node)
	}

	for _, name := range cluster.Spec.ControlPlane.Nodes {
		if err := patch(ctx, name); err != nil {
			return fmt.Errorf("KubeControlPlane %+v", err)
		}
	}

	return nil
}

// patchMachineNode updates a node's cluster ownership and owner references
func (r *ClusterReconciler) patchMachineNode(ctx context.Context, cluster *v1.Cluster, node *v1.Node) error {
	if cluster.DeletionTimestamp.IsZero() {
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		node.Spec.Cluster = &cluster.Name
	} else if cluster.Status.ControlPlaneStatus.Phase == v1.DeletedPhase {
		node.Spec.Cluster = nil
		klog.Infof("nodes %s remove owner references", node.Name)
	} else {
		return nil
	}

	return r.Update(ctx, node)
}

// generateSSHSecret creates an SSH secret for cluster access if it doesn't exist
func (r *ClusterReconciler) generateSSHSecret(ctx context.Context, cluster *v1.Cluster) error {
	secret := new(corev1.Secret)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      cluster.Name,
	}, secret)

	if err == nil || !errors.IsNotFound(err) {
		return err
	}

	username, private, pub, err := r.createSSHKeyPair(ctx, cluster)
	if err != nil {
		return err
	}

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cluster.Name,
			Namespace:       common.PrimusSafeNamespace,
			OwnerReferences: []metav1.OwnerReference{createKubernetesClusterOwnerReference(cluster)},
		},
		Data: map[string][]byte{
			utils.Username:     []byte(username),
			utils.Authorize:    private,
			utils.AuthorizePub: pub,
		},
		Type: corev1.SecretTypeOpaque,
	}

	err = r.Create(ctx, secret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}

	return nil
}

// createSSHKeyPair creates SSH key pair for the cluster
func (r *ClusterReconciler) createSSHKeyPair(ctx context.Context, cluster *v1.Cluster) (string, []byte, []byte, error) {
	node := new(v1.Node)
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Spec.ControlPlane.Nodes[0]}, node)
	if err != nil {
		return "", nil, nil, err
	}

	username, err := r.ClusterBaseReconciler.getUsername(ctx, node, cluster)
	if err != nil {
		return "", nil, nil, err
	}

	private, pub, err := secure.MakeSSHKeyPair()
	if err != nil {
		return "", nil, nil, err
	}

	return username, private, pub, nil
}

// clearPods cleans up succeeded pods that are older than one hour
func (r *ClusterReconciler) clearPods(ctx context.Context, cluster *v1.Cluster) error {
	labelSelector := client.MatchingLabels{v1.ClusterManageClusterLabel: cluster.Name}
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return err
	}

	for _, pod := range list.Items {
		if pod.Status.Phase != corev1.PodSucceeded {
			continue
		}

		if time.Now().UTC().After(pod.CreationTimestamp.Add(time.Hour)) {
			if err = r.Delete(ctx, &pod); err != nil {
				klog.Errorf("cluster %s delete pod failed %+v", cluster.Name, err)
			}
		}
	}

	return nil
}

// guaranteeService creates the Kubernetes service and endpoints for the cluster
func (r *ClusterReconciler) guaranteeService(ctx context.Context, cluster *v1.Cluster) error {
	if cluster.Status.ControlPlaneStatus.Phase != v1.ReadyPhase && cluster.Status.ControlPlaneStatus.Phase != v1.CreatedPhase {
		return nil
	}

	nodes, err := r.getControllerPlaneNodes(ctx, cluster)
	if err != nil {
		return err
	}

	if len(nodes) == 0 {
		return nil
	}

	if err = r.guaranteeEndpoints(ctx, cluster, nodes); err != nil {
		return err
	}

	return r.guaranteeServiceResource(ctx, cluster)
}

// guaranteeEndpoints creates the endpoints resource for the cluster
func (r *ClusterReconciler) guaranteeEndpoints(ctx context.Context, cluster *v1.Cluster, nodes []*v1.Node) error {
	endpoint := new(corev1.Endpoints)
	err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, endpoint)

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if err == nil {
		return nil
	}

	address := make([]corev1.EndpointAddress, 0, len(nodes))
	for _, node := range nodes {
		address = append(address, corev1.EndpointAddress{IP: node.Spec.PrivateIP})
	}

	endpoint = &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cluster.Name,
			Namespace:       common.PrimusSafeNamespace,
			OwnerReferences: []metav1.OwnerReference{createKubernetesClusterOwnerReference(cluster)},
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: address,
				Ports: []corev1.EndpointPort{
					{
						Name:     "https",
						Port:     DefaultApiserverPort,
						Protocol: "TCP",
					},
				},
			},
		},
	}

	err = r.Create(ctx, endpoint)
	if err != nil {
		return fmt.Errorf("create cluster endpoint failed %+v", err)
	}

	return nil
}

// guaranteeServiceResource creates the service resource for the cluster
func (r *ClusterReconciler) guaranteeServiceResource(ctx context.Context, cluster *v1.Cluster) error {
	service := new(corev1.Service)
	err := r.Get(ctx, types.NamespacedName{
		Name:      cluster.Name,
		Namespace: common.PrimusSafeNamespace,
	}, service)

	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if err == nil {
		return nil
	}

	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cluster.Name,
			Namespace:       common.PrimusSafeNamespace,
			OwnerReferences: []metav1.OwnerReference{createKubernetesClusterOwnerReference(cluster)},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "https",
				Protocol:   "TCP",
				Port:       DefaultHttpServiePort,
				TargetPort: intstr.IntOrString{IntVal: DefaultApiserverPort},
			}},
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}

	if err = r.Create(ctx, service); err != nil {
		return fmt.Errorf("create cluster service failed %+v", err)
	}

	return nil
}
