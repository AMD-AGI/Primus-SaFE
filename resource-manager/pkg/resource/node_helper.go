/*
   Copyright © 01.AI Co., Ltd. 2023-2024. All rights reserved.
*/

package resource

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	xcscrypto "github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
)

const (
	finalizer      = "xcs.storage.controller"
	addonFinalizer = "xcs.addon.controller"
)

const (
	XCSKubernetesFinalizer    = "xcs.kubernetes.finalizer"
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
	Controllers   []*v1.MachineNode
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

func generateWorkerPod(action v1.ClusterManageAction, cluster *v1.KubernetesCluster, username, cmd, image, config string, hostsContent *HostTemplateContent) *corev1.Pod {
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

	if cluster.Spec.KubeApiServerArgs != nil && len(cluster.Spec.KubeApiServerArgs) > 0 {
		replace := "kube_kubeadm_apiserver_extra_args:"
		for k, v := range cluster.Spec.KubeApiServerArgs {
			replace = fmt.Sprintf("%s \n  %s: %s", replace, k, v)
		}
		cmd = fmt.Sprintf("sed -i \"/^kube_kubeadm_apiserver_extra_args: /d\" roles/kubernetes/control-plane/defaults/main/main.yml && echo \"%s\" >> roles/kubernetes/control-plane/defaults/main/main.yml && %s", replace, cmd)
	}

	sshSecretName := cluster.Name
	if cluster.Spec.SSHSecret != nil {
		sshSecretName = cluster.Spec.SSHSecret.Name
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.XcsNamespace,
			Labels: map[string]string{
				v1.KubernetesManageActionLabel:  string(action),
				v1.KubernetesManageClusterLabel: cluster.Name,
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
	if cluster.Spec.ImageSecret != nil {
		pod.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: cluster.Spec.ImageSecret.Name,
			},
		}
	}
	return pod
}

func generateScaleWorkerPod(action v1.ClusterManageAction, cluster *v1.KubernetesCluster, node *v1.MachineNode, usename, cmd, image, config string, hostsContent *HostTemplateContent) *corev1.Pod {
	pod := generateWorkerPod(action, cluster, usename, cmd, image, config, hostsContent)
	name := fmt.Sprintf("%s-%s", cluster.Name, node.Name)
	if len(name) > 58 {
		name = name[:58]
	}
	name = fmt.Sprintf("%s-%s", name, action)
	pod.Name = names.SimpleNameGenerator.GenerateName(name + "-")
	pod.Labels[v1.KubernetesManageNodeLabel] = node.Name
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

func GetKubeSprayEnv(cluster *v1.KubernetesCluster) string {
	cmd := ""
	if cluster.Spec.KubernetesVersion != nil {
		cmd = fmt.Sprintf("%s -e kube_version=%s", cmd, *cluster.Spec.KubernetesVersion)
	}
	if cluster.Spec.KubePodsSubnet != nil {
		cmd = fmt.Sprintf("%s -e kube_pods_subnet=%s", cmd, *cluster.Spec.KubePodsSubnet)
	}
	if cluster.Spec.KubeServiceAddress != nil {
		cmd = fmt.Sprintf("%s -e kube_service_addresses=%s", cmd, *cluster.Spec.KubeServiceAddress)
	}
	if cluster.Spec.KubeProxyMode != nil {
		cmd = fmt.Sprintf("%s -e kube_proxy_mode=%s", cmd, *cluster.Spec.KubeProxyMode)
	}
	if cluster.Spec.NodeLocalDNSIP != nil {
		cmd = fmt.Sprintf("%s -e nodelocaldns_ip=%s", cmd, *cluster.Spec.NodeLocalDNSIP)
	}
	if cluster.Spec.KubeletLogFilesMaxSize != nil {
		cmd = fmt.Sprintf("%s -e kubelet_logfiles_max_size=%s", cmd, cluster.Spec.KubeletLogFilesMaxSize.String())
	}

	if cluster.Spec.KubeNetworkNodePrefix != nil {
		cmd = fmt.Sprintf("%s -e kube_network_node_prefix=%d", cmd, *cluster.Spec.KubeNetworkNodePrefix)
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

func getKubesprayImage(cluster *v1.KubernetesCluster) string {
	if cluster.Spec.KubeSprayImage != nil {
		return *cluster.Spec.KubeSprayImage
	}
	return "quay.io/kubespray/kubespray:v2.24.0"
}

func addOwnerReferences(references []metav1.OwnerReference, cluster *v1.KubernetesCluster) []metav1.OwnerReference {
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
	if err = session.Run("hostname"); err != nil {
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

func machineStatus(node *v1.Node) v1.MachineNodeStatusType {
	for _, v := range node.Status.UnmanagedStatus {
		if v.Status != v1.CommandSucceeded {
			return v1.MachineNodeUnmanaged
		}
	}
	if node.Status.HostName == "" {
		return v1.MachineNodeNotReady
	}
	return v1.MachineNodeReady
}

func getNodeLabelsString(machine *v1.MachineNode) (string, bool) {
	if machine.Labels == nil {
		return "", false
	}
	labels := machine.Labels
	delete(machine.Labels, v1.KubernetesManageNodeClusterLabel)
	if machine.Spec.NodeFlavor != nil {
		labels[v1.NodeFlavorLabel] = machine.Spec.NodeFlavor.Name
	}
	l, err := json.Marshal(labels)
	if err != nil {
		return "", false
	}
	return string(l), true
}

func createKubernetesClusterOwnerReference(cluster *v1.KubernetesCluster) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         cluster.APIVersion,
		Kind:               cluster.Kind,
		Name:               cluster.Name,
		UID:                cluster.UID,
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}
}

func newKubernetesConfig(cluster *v1.KubernetesCluster) (*rest.Config, error) {
	if cluster.Status.Phase != v1.ReadyPhase || cluster.Status.CAData == "" || cluster.Status.KeyData == "" || cluster.Status.CertData == "" {
		return nil, fmt.Errorf("cluster is not ready")
	}
	if len(cluster.Status.Endpoints) == 0 {
		return nil, nil
	}
	crypto := xcscrypto.Instance()
	config := &rest.Config{
		Host: fmt.Sprintf("https://%s.%s.svc", cluster.Name, common.XcsNamespace),
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	certData, err := crypto.Decrypt(cluster.Status.CertData)
	if err != nil {
		return nil, err
	}
	config.TLSClientConfig.CertData, err = base64.StdEncoding.DecodeString(certData)
	if err != nil {
		return nil, err
	}
	keyData, err := crypto.Decrypt(cluster.Status.KeyData)
	if err != nil {
		return nil, err
	}
	config.TLSClientConfig.KeyData, err = base64.StdEncoding.DecodeString(keyData)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func newKubernetesClientSet(cluster *v1.KubernetesCluster) (kubernetes.Interface, error) {
	config, err := newKubernetesConfig(cluster)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func isReadyMachineNode(node *v1.MachineNode) bool {
	if node.Status.KubernetesStatus.Status == "" {
		return false
	}
	if node.Status.KubernetesStatus.Status == v1.MachineNodeNotReady {
		return false
	}
	if node.Status.KubernetesStatus.Status == v1.MachineNodeManagedFailed {
		return false
	}
	return true
}

func isCommandSuccessful(status []v1.CommandStatus, name string) bool {
	for _, v := range status {
		if v.Name == name && v.Status == v1.CommandSucceeded {
			return true
		}
	}
	return false
}

func getUsername(ctx context.Context, cli client.Client, node *v1.Node, cluster *v1.Cluster) (string, error) {
	if cluster.Spec.SSHSecret != nil {
		secret := new(corev1.Secret)
		err := cli.Get(ctx, types.NamespacedName{
			Namespace: cluster.Spec.SSHSecret.Namespace,
			Name:      cluster.Spec.SSHSecret.Name,
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
		err := cli.Get(ctx, types.NamespacedName{Name: node.Spec.SSHSecret.Name, Namespace: node.Spec.SSHSecret.Namespace}, secret)
		if err != nil {
			return "", err
		}
		if data, ok := secret.Data[Username]; ok {
			return string(data), nil
		}
	}
	secret := new(corev1.Secret)
	err := cli.Get(ctx, types.NamespacedName{
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
