/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"context"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BaseReconciler struct {
	client.Client
}

func (r *BaseReconciler) getUsername(ctx context.Context, node *v1.Node, cluster *v1.Cluster) (string, error) {
	if cluster.Spec.ControlPlane.SSHSecret != nil {
		secret := new(corev1.Secret)
		err := r.Get(ctx, types.NamespacedName{
			Namespace: cluster.Spec.ControlPlane.SSHSecret.Namespace,
			Name:      cluster.Spec.ControlPlane.SSHSecret.Name,
		}, secret)
		if err != nil {
			return "", err
		}
		if data, ok := secret.Data[Username]; ok {
			return string(data), nil
		}
	}
	if node.Spec.SSHSecret != nil {
		secret := new(corev1.Secret)
		err := r.Get(ctx, types.NamespacedName{Name: node.Spec.SSHSecret.Name, Namespace: node.Spec.SSHSecret.Namespace}, secret)
		if err != nil {
			return "", err
		}
		if data, ok := secret.Data[Username]; ok {
			return string(data), nil
		} else {
			return "root", nil
		}
	}
	secret := new(corev1.Secret)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      cluster.Name,
	}, secret)
	if err != nil {
		return "", err
	}
	username := "root"
	if data, ok := secret.Data[Username]; ok {
		username = string(data)
	}
	return username, nil
}

func (r *BaseReconciler) getNode(ctx context.Context, nodes *v1.NodeList, name string) (*v1.Node, error) {
	for _, n := range nodes.Items {
		if n.Name == name {
			return n.DeepCopy(), nil
		}
	}
	node := new(v1.Node)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      name,
	}, node)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (r *BaseReconciler) generateHosts(ctx context.Context, cluster *v1.Cluster, worker *v1.Node) (*HostTemplateContent, error) {
	nodes := new(v1.NodeList)
	if err := r.List(ctx, nodes); err != nil {
		return nil, err
	}
	controllers := make([]*v1.Node, 0, len(cluster.Spec.ControlPlane.Nodes))
	for _, v := range cluster.Spec.ControlPlane.Nodes {
		node, err := r.getNode(ctx, nodes, v)
		if err != nil {
			return nil, err
		}
		if !isReadyMachineNode(node) {
			klog.Infof("machine node %s not ready status is %s", node.Name, node.Status.MachineStatus.Phase)
			continue
		}
		controllers = append(controllers, node)
	}

	if len(controllers) != len(cluster.Spec.ControlPlane.Nodes) {
		return nil, nil
	}

	hostsContent := &HostTemplateContent{
		NodeAndIP:     make([]string, 0, len(controllers)+1),
		MasterName:    make([]string, 0, len(controllers)),
		NodeName:      make([]string, 0, 1),
		EtcdNodeName:  make([]string, 0, len(controllers)),
		Hosts:         make([]string, 0, len(controllers)+1),
		PodHostsAlias: map[string]string{},
		Labels:        map[string]string{},
		ClusterName:   cluster.Name,
		Controllers:   controllers,
	}
	if cluster.Spec.ClusterID != nil {
		hostsContent.ClusterID = *cluster.Spec.ClusterID
	} else {
		hostsContent.ClusterID = "1.0.0.1"
	}
	count := 0
	for _, machine := range controllers {
		hostname := machine.Status.MachineStatus.HostName
		publicIP := machine.Spec.PublicIP
		if publicIP == "" {
			publicIP = machine.Spec.PrivateIP
		}
		username, err := r.getUsername(ctx, machine, cluster)
		if err != nil {
			return nil, err
		}
		nodeAndIp := fmt.Sprintf("%s ansible_host=%s ip=%s ansible_ssh_user=%s", hostname, publicIP, machine.Spec.PrivateIP, username)
		hostsContent.MasterName = append(hostsContent.MasterName, hostname)
		hostsContent.EtcdNodeName = append(hostsContent.EtcdNodeName, hostname)
		hostsContent.NodeAndIP = append(hostsContent.NodeAndIP, nodeAndIp)
		if hostname != publicIP {
			hostsContent.Hosts = append(hostsContent.Hosts, fmt.Sprintf("%s %s", publicIP, hostname))
			hostsContent.PodHostsAlias[hostname] = publicIP
		}
		if l, ok := getNodeLabelsString(machine); ok {
			hostsContent.Labels[machine.Name] = l
		}
		count++
	}
	if worker != nil {
		node, err := r.getNode(ctx, nodes, worker.Name)
		if err != nil {
			return nil, err
		}
		hostname := node.Status.MachineStatus.HostName
		publicIP := node.Spec.PublicIP
		if publicIP == "" {
			publicIP = node.Spec.PrivateIP
		}
		username, err := r.getUsername(ctx, node, cluster)
		if err != nil {
			return nil, err
		}
		nodeAndIp := fmt.Sprintf("%s ansible_host=%s ip=%s ansible_ssh_user=%s", hostname, publicIP, node.Spec.PrivateIP, username)
		hostsContent.NodeName = append(hostsContent.NodeName, hostname)
		hostsContent.NodeAndIP = append(hostsContent.NodeAndIP, nodeAndIp)
		if hostname != publicIP {
			hostsContent.Hosts = append(hostsContent.Hosts, fmt.Sprintf("%s %s", publicIP, hostname))
			hostsContent.PodHostsAlias[hostname] = publicIP
		}

		if l, ok := getNodeLabelsString(node); ok {
			hostsContent.Labels[node.Name] = l
		}
		count++
	}
	if len(hostsContent.NodeName) == 0 {
		hostsContent.NodeName = append(hostsContent.NodeName, hostsContent.MasterName...)
	}
	return hostsContent, nil
}

func (r *BaseReconciler) ensureHostsConfigMapCreated(ctx context.Context, name string, owner metav1.OwnerReference, hostsContent *HostTemplateContent) (*corev1.ConfigMap, error) {
	kebesprayHostData := &strings.Builder{}
	tmpl := template.Must(template.New("").Parse(kubesprayHostsTemplate))
	if err := tmpl.Execute(kebesprayHostData, hostsContent); err != nil {
		return nil, err
	}

	hostData := &strings.Builder{}

	tmpl = template.Must(template.New("").Parse(clusterHostsTemplate))
	if err := tmpl.Execute(hostData, hostsContent); err != nil {
		return nil, err
	}
	cm := new(corev1.ConfigMap)
	err := r.Get(ctx, types.NamespacedName{
		Namespace: common.PrimusSafeNamespace,
		Name:      name,
	}, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		cm = &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: common.PrimusSafeNamespace,
				OwnerReferences: []metav1.OwnerReference{
					owner,
				},
			},
			Data: map[string]string{
				HostsYaml: strings.TrimSpace(kebesprayHostData.String()),
				Hosts:     strings.TrimSpace(hostData.String()),
			},
		}
		if err := r.Client.Create(ctx, cm); err != nil {
			return nil, err
		}
	} else {
		c := client.MergeFrom(cm.DeepCopy())
		cm.Data[HostsYaml] = strings.TrimSpace(kebesprayHostData.String())
		cm.Data[Hosts] = strings.TrimSpace(hostData.String())
		err = r.Patch(ctx, cm, c)
		if err != nil {
			return nil, err
		}
	}
	klog.Info("hostsContent.Hosts length", len(hostsContent.Hosts))
	return cm, nil
}

func (r *BaseReconciler) ensurePod(ctx context.Context, clusterName string, action v1.ClusterManageAction) (*corev1.Pod, error) {
	labelSelector := client.MatchingLabels{v1.ClusterManageActionLabel: string(action), v1.ClusterManageClusterLabel: clusterName}
	list := new(corev1.PodList)
	err := r.List(ctx, list, client.InNamespace(common.PrimusSafeNamespace), labelSelector)
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 0 {
		return &list.Items[0], nil
	}
	return nil, nil
}

const (
	finalizer = "storage.controller"
)

//go:embed cluster_hosts.template
var clusterHostsTemplate string

//go:embed kubespray_hosts.template
var kubesprayHostsTemplate string

const (
	KubernetesFinalizer       = "kubernetes.finalizer"
	ProvisionedKubeConfigPath = "/etc/kubernetes/admin.conf"
	Username                  = "username"
	Password                  = "password"
	Root                      = "root"
	ClusterKubeSprayHosts     = "cluster-kube-spray-hosts"
	ClusterSecret             = "cluster-secret"
	Hosts                     = "hosts"
	HostsYaml                 = "hosts.yaml"
	Authorize                 = "authorize"
	AuthorizePub              = "authorize.pub"
	Requeue                   = time.Second * 30
	Zero                      = time.Duration(0)
	HarborCA                  = "harbor-ca"
)

type HostTemplateContent struct {
	NodeAndIP     []string
	MasterName    []string
	NodeName      []string
	EtcdNodeName  []string
	Hosts         []string
	PodHostsAlias map[string]string
	Labels        map[string]string
	ClusterName   string
	ClusterID     string
	Controllers   []*v1.Node
}

func getSHHConfig(secret *corev1.Secret) (*ssh.ClientConfig, error) {
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
	}

	if sshPrivateKeyData, ok := secret.Data[Authorize]; ok {
		signer, err := ssh.ParsePrivateKey(sshPrivateKeyData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SSH private key: %v", err)
		}
		sshConfig.Auth = append(sshConfig.Auth, ssh.PublicKeys(signer))
	} else if password, ok := secret.Data[Password]; ok {
		sshConfig.Auth = append(sshConfig.Auth, ssh.Password(string(password)))
	} else {
		return nil, fmt.Errorf("ssh private key or password not found in secret")
	}
	return sshConfig, nil
}

func generateWorkerPod(action v1.ClusterManageAction, cluster *v1.Cluster, username, cmd, image, config string, hostsContent *HostTemplateContent) *corev1.Pod {
	basePodName := cluster.Name + "-" + string(action)
	name := names.SimpleNameGenerator.GenerateName(basePodName + "-")
	hostsAlias := make([]corev1.HostAlias, 0, len(hostsContent.PodHostsAlias))
	for hostname, ip := range hostsContent.PodHostsAlias {
		hostsAlias = append(hostsAlias, corev1.HostAlias{
			IP: ip,
			Hostnames: []string{
				hostname,
			},
		})
	}
	// root 0400 other 0644
	mode := pointer.Int32(0644)
	if username == Root {
		mode = pointer.Int32(0400)
	}

	if cluster.Spec.ControlPlane.KubeApiServerArgs != nil && len(cluster.Spec.ControlPlane.KubeApiServerArgs) > 0 {
		replace := "kube_kubeadm_apiserver_extra_args:"
		for k, v := range cluster.Spec.ControlPlane.KubeApiServerArgs {
			replace = fmt.Sprintf("%s \n  %s: %s", replace, k, v)
		}
		cmd = fmt.Sprintf("sed -i \"/^kube_kubeadm_apiserver_extra_args: /d\" roles/kubernetes/control-plane/defaults/main/main.yml && echo \"%s\" >> roles/kubernetes/control-plane/defaults/main/main.yml && %s", replace, cmd)
	}

	sshSecretName := cluster.Name
	if cluster.Spec.ControlPlane.SSHSecret != nil {
		sshSecretName = cluster.Spec.ControlPlane.SSHSecret.Name
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
			Labels: map[string]string{
				v1.ClusterManageActionLabel:  string(action),
				v1.ClusterManageClusterLabel: cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         cluster.APIVersion,
					Kind:               cluster.Kind,
					Name:               cluster.Name,
					UID:                cluster.UID,
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    string(action),
					Command: []string{"/bin/bash", "-c"},
					Args:    []string{cmd},
					Image:   image,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      ClusterKubeSprayHosts,
							ReadOnly:  true,
							MountPath: "kubespray/hosts",
						},
						{
							Name:      ClusterSecret,
							ReadOnly:  true,
							MountPath: "kubespray/.ssh",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: ClusterKubeSprayHosts,
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: config,
							},
							Items: []corev1.KeyToPath{
								{
									Key:  HostsYaml,
									Path: HostsYaml,
								},
								{
									Key:  Hosts,
									Path: Hosts,
								},
							},
						},
					},
				},
				{
					Name: ClusterSecret,
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  sshSecretName,
							DefaultMode: mode,
							Items: []corev1.KeyToPath{
								{
									Key:  Authorize,
									Path: Authorize,
								},
							},
						},
					},
				},
			},
			HostAliases:   hostsAlias,
			RestartPolicy: corev1.RestartPolicyNever,
		},
		Status: corev1.PodStatus{},
	}
	if cluster.Spec.ControlPlane.ImageSecret != nil {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: cluster.Spec.ControlPlane.ImageSecret.Name,
			},
		}
	}
	return pod
}

func generateScaleWorkerPod(action v1.ClusterManageAction, cluster *v1.Cluster, node *v1.Node, usename, cmd, image, config string, hostsContent *HostTemplateContent) *corev1.Pod {
	pod := generateWorkerPod(action, cluster, usename, cmd, image, config, hostsContent)
	name := fmt.Sprintf("%s-%s", cluster.Name, node.Name)
	if len(name) > 58 {
		name = name[:58]
	}
	name = fmt.Sprintf("%s-%s", name, action)
	pod.Name = names.SimpleNameGenerator.GenerateName(name + "-")
	pod.Labels[v1.ClusterManageNodeLabel] = node.Name
	pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
		APIVersion: node.APIVersion,
		Kind:       node.Kind,
		Name:       node.Name,
		UID:        node.UID,
	})
	return pod
}
func GetKubeSprayCreateCMD(user, env string) string {
	cmd := fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s cluster.yml --become-user=root %s -b -vvv", Authorize, env)
	if user == "" || user == "root" {
		return cmd
	}
	return fmt.Sprintf("groupadd -r kubespray && useradd -r -g kubespray %s && mkdir -p /home/%s && chmod -R 777 /home/%s && su %s -c '%s'",
		user, user, user, user, cmd)
}

func GetKubeSprayScaleUpCMD(user, node, env string) string {
	cmd := fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s scale.yml --limit=%s %s --become-user=root -b -vvv", Authorize, node, env)
	if user == "" || user == "root" {
		return cmd
	}
	return fmt.Sprintf("groupadd -r kubespray && useradd -r -g kubespray %s && mkdir -p /home/%s && chmod -R 777 /home/%s && su %s -c '%s'",
		user, user, user, user, cmd)
}

func GetKubeSprayScaleDownCMD(user, node, env string) string {
	cmd := fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s remove-node.yml -e node=%s -e skip_confirmation=yes -e reset_nodes=true -e allow_ungraceful_removal=false %s --become-user=root -b -vvv", Authorize, node, env)
	if user == "" || user == "root" {
		return cmd
	}
	return fmt.Sprintf("groupadd -r kubespray && useradd -r -g kubespray %s && mkdir -p /home/%s && chmod -R 777 /home/%s && su %s -c '%s'",
		user, user, user, user, cmd)
}

func GetKubeSprayHostsCMD(user string) string {
	cmd := fmt.Sprintf("ansible all -i hosts/hosts.yaml --private-key .ssh/%s -m copy -a \"src=inventory/hosts dest=/etc/hosts mode=u+x\" --become-user=root -b -vvv", Authorize)
	if user == "" || user == "root" {
		return cmd
	}
	return fmt.Sprintf("groupadd -r kubespray && useradd -r -g kubespray %s && mkdir -p /home/%s && chmod -R 777 /home/%s && su %s -c '%s'",
		user, user, user, user, cmd)
}

func GetKubeSprayEnv(cluster *v1.Cluster) string {
	cmd := ""
	if cluster.Spec.ControlPlane.KubeVersion != nil {
		cmd = fmt.Sprintf("%s -e kube_version=%s", cmd, *cluster.Spec.ControlPlane.KubeVersion)
	}
	if cluster.Spec.ControlPlane.KubePodsSubnet != nil {
		cmd = fmt.Sprintf("%s -e kube_pods_subnet=%s", cmd, *cluster.Spec.ControlPlane.KubePodsSubnet)
	}
	if cluster.Spec.ControlPlane.KubeServiceAddress != nil {
		cmd = fmt.Sprintf("%s -e kube_service_addresses=%s", cmd, *cluster.Spec.ControlPlane.KubeServiceAddress)
	}
	if cluster.Spec.ControlPlane.KubeProxyMode != nil {
		cmd = fmt.Sprintf("%s -e kube_proxy_mode=%s", cmd, *cluster.Spec.ControlPlane.KubeProxyMode)
	}
	if cluster.Spec.ControlPlane.KubeNetworkPlugin != nil {
		cmd = fmt.Sprintf("%s -e kube_network_plugin=%s", cmd, *cluster.Spec.ControlPlane.KubeNetworkPlugin)
	}
	if cluster.Spec.ControlPlane.NodeLocalDNSIP != nil {
		cmd = fmt.Sprintf("%s -e nodelocaldns_ip=%s", cmd, *cluster.Spec.ControlPlane.NodeLocalDNSIP)
	}
	if cluster.Spec.ControlPlane.KubeletLogFilesMaxSize != nil {
		cmd = fmt.Sprintf("%s -e kubelet_logfiles_max_size=%s", cmd, cluster.Spec.ControlPlane.KubeletLogFilesMaxSize.String())
	}

	if cluster.Spec.ControlPlane.KubeNetworkNodePrefix != nil {
		cmd = fmt.Sprintf("%s -e kube_network_node_prefix=%d", cmd, *cluster.Spec.ControlPlane.KubeNetworkNodePrefix)
	}
	cmd = fmt.Sprintf("%s -e auto_renew_certificates=true -e nginx_image_repo=public.ecr.aws/docker/library/nginx", cmd)
	return cmd
}

func GetKubeSprayResetCMD(user, env string) string {
	cmd := fmt.Sprintf("ansible-playbook -i hosts/hosts.yaml --private-key .ssh/%s reset.yml -e reset_confirmation=yes %s --become-user=root -b -vvv", Authorize, env)
	if user == "" || user == "root" {
		return cmd
	}
	return fmt.Sprintf("groupadd -r kubespray && useradd -r -g kubespray %s && mkdir -p /home/%s && chmod -R 777 /home/%s && su %s -c '%s'",
		user, user, user, user, cmd)
}

func getKubesprayImage(cluster *v1.Cluster) string {
	if cluster.Spec.ControlPlane.KubeSprayImage != nil && *cluster.Spec.ControlPlane.KubeSprayImage != "" {
		return *cluster.Spec.ControlPlane.KubeSprayImage
	}
	return "quay.io/kubespray/kubespray:v2.24.0"
}

func addOwnerReferences(references []metav1.OwnerReference, cluster *v1.Cluster) []metav1.OwnerReference {
	for _, r := range references {
		if r.UID == cluster.UID {
			return references
		}
	}
	references = append(references, metav1.OwnerReference{
		APIVersion: cluster.APIVersion,
		Kind:       cluster.Kind,
		Name:       cluster.Name,
		UID:        cluster.UID,
	})
	return references
}

func removeOwnerReferences(references []metav1.OwnerReference, uid types.UID) []metav1.OwnerReference {
	newReferences := make([]metav1.OwnerReference, 0, len(references))
	for k, r := range references {
		if r.UID != uid {
			newReferences = append(newReferences, references[k])
		}
	}
	return newReferences
}

func getHostname(conn *ssh.Client) (string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run("hostname"); err != nil {
		return "", fmt.Errorf("failed get hostname: %v", err)
	}
	return strings.Replace(b.String(), "\n", "", -1), nil
}

func setHostname(conn *ssh.Client, hostname string) (string, error) {
	session, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(fmt.Sprintf("sudo hostnamectl set-hostname %s && hostname", hostname)); err != nil {
		return "", fmt.Errorf("failed get hostname: %v", err)
	}
	return strings.Replace(b.String(), "\n", "", -1), nil
}

func getNodeLabelsString(node *v1.Node) (string, bool) {
	if node.Labels == nil {
		return "", false
	}
	labels := node.Labels
	delete(node.Labels, v1.ClusterManageNodeClusterLabel)
	if node.Spec.NodeFlavor != nil {
		labels[v1.NodeFlavorLabel] = node.Spec.NodeFlavor.Name
	}
	l, err := json.Marshal(labels)
	if err != nil {
		return "", false
	}
	return string(l), true
}

func createKubernetesClusterOwnerReference(cluster *v1.Cluster) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         cluster.APIVersion,
		Kind:               cluster.Kind,
		Name:               cluster.Name,
		UID:                cluster.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}
}

func newKubernetesConfig(cluster *v1.Cluster) (*rest.Config, error) {
	if cluster.Status.ControlePlaneStatus.Phase != v1.ReadyPhase || cluster.Status.ControlePlaneStatus.CAData == "" ||
		cluster.Status.ControlePlaneStatus.KeyData == "" || cluster.Status.ControlePlaneStatus.CertData == "" {
		return nil, fmt.Errorf("cluster is not ready")
	}
	if len(cluster.Status.ControlePlaneStatus.Endpoints) == 0 {
		return nil, nil
	}
	crypto := crypto.Instance()
	config := &rest.Config{
		Host: fmt.Sprintf("https://%s.%s.svc", cluster.Name, ""),
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	certData, err := crypto.Decrypt(cluster.Status.ControlePlaneStatus.CertData)
	if err != nil {
		return nil, err
	}
	config.TLSClientConfig.CertData, err = base64.StdEncoding.DecodeString(certData)
	if err != nil {
		return nil, err
	}
	keyData, err := crypto.Decrypt(cluster.Status.ControlePlaneStatus.KeyData)
	if err != nil {
		return nil, err
	}
	config.TLSClientConfig.KeyData, err = base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func newKubernetesClientSet(cluster *v1.Cluster) (kubernetes.Interface, error) {
	config, err := newKubernetesConfig(cluster)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func isReadyMachineNode(node *v1.Node) bool {
	if node.Status.MachineStatus.Phase == "" {
		return false
	}
	if node.Status.MachineStatus.Phase == v1.NodeNotReady {
		return false
	}
	if node.Status.MachineStatus.Phase == v1.NodeManagedFailed {
		return false
	}
	return true
}
