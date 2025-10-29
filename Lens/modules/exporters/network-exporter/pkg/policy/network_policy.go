package policy

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/kube"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NetworkPolicy struct {
	InternalHosts     []string `json:"internal_hosts"`
	K8SPod            []string `json:"k8s_pod"`
	K8SSvc            []string `json:"k8s_svc"`
	Dns               []string `json:"dns"`
	AbnormalBlackList []string `json:"abnormal_black_list"`
	AbnormalWhiteList []string `json:"abnormal_white_list"`
	Localhost         []string `json:"localhost"`
}

var defaultPolicy = &NetworkPolicy{
	InternalHosts: []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	},
}

func LoadDefaultPolicy(ctx context.Context, client client.Client) error {
	podCidr, svcCidr, err := kube.GetCIDRForKubeadm(ctx, client)
	if err != nil {
		return err
	}
	defaultPolicy.K8SPod = []string{podCidr}
	defaultPolicy.K8SSvc = []string{svcCidr}
	nodeLocalDns, err := util.GetNodeLocalDNS()
	if err == nil && nodeLocalDns != "" {
		defaultPolicy.Dns = []string{fmt.Sprintf("%s/32", nodeLocalDns)}
	}
	return nil
}

func GetDefaultPolicy() NetworkPolicy {
	return *defaultPolicy
}
