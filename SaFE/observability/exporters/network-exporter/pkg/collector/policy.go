// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"net"
	"os"
	"strings"

	"github.com/yl2chen/cidranger"
)

const (
	IPSourceK8sPod        = "k8sPod"
	IPSourceK8sSvc        = "k8sSvc"
	IPSourceDNS           = "dns"
	IPSourceDocker        = "docker"
	IPSourceInternalHosts = "internalHosts"
	IPSourceExternalHosts = "externalHosts"
	IPSourceUnknown       = "unknown"
	IPSourceError         = "error"
	IPSourceLocalhost     = "localhost"
)

type NetworkPolicy struct {
	InternalHosts []string `json:"internal_hosts"`
	K8SPod        []string `json:"k8s_pod"`
	K8SSvc        []string `json:"k8s_svc"`
	Dns           []string `json:"dns"`
	Localhost     []string `json:"localhost"`
}

type policyRanger struct {
	localHostsRanger    cidranger.Ranger
	k8sPodRanger        cidranger.Ranger
	k8sSvcRanger        cidranger.Ranger
	dnsRanger           cidranger.Ranger
	dockerRanger        cidranger.Ranger
	internalHostsRanger cidranger.Ranger
}

func (p *policyRanger) match(ip string) (string, bool, error) {
	netIP := net.ParseIP(ip)
	if netIP == nil {
		return "", false, nil
	}
	if ok, err := p.k8sPodRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if ok {
		return IPSourceK8sPod, true, nil
	}
	if ok, err := p.k8sSvcRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if ok {
		return IPSourceK8sSvc, true, nil
	}
	if ok, err := p.dnsRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if ok {
		return IPSourceDNS, true, nil
	}
	if p.dockerRanger != nil {
		if ok, err := p.dockerRanger.Contains(netIP); err != nil {
			return "", false, err
		} else if ok {
			return IPSourceDocker, ok, nil
		}
	}
	if ok, err := p.localHostsRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if ok {
		return IPSourceLocalhost, true, nil
	}
	if ok, err := p.internalHostsRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if ok {
		return IPSourceInternalHosts, ok, nil
	}
	return IPSourceExternalHosts, true, nil
}

func (p *policyRanger) setDockerRanger(ipRange []string) {
	dockerRanger := cidranger.NewPCTrieRanger()
	for _, ip := range ipRange {
		_, network, _ := net.ParseCIDR(ip)
		if network != nil {
			dockerRanger.Insert(cidranger.NewBasicRangerEntry(*network))
		}
	}
	p.dockerRanger = dockerRanger
}

func (p *policyRanger) loadFromConfig(cfg NetworkPolicy) {
	k8sPodRanger := cidranger.NewPCTrieRanger()
	k8sSvcRanger := cidranger.NewPCTrieRanger()
	dnsRanger := cidranger.NewPCTrieRanger()
	internalHostsRanger := cidranger.NewPCTrieRanger()
	localhostRanger := cidranger.NewPCTrieRanger()

	for _, s := range cfg.Localhost {
		_, network, _ := net.ParseCIDR(s)
		if network != nil {
			localhostRanger.Insert(cidranger.NewBasicRangerEntry(*network))
		}
	}
	for _, pod := range cfg.K8SPod {
		_, network, _ := net.ParseCIDR(pod)
		if network != nil {
			k8sPodRanger.Insert(cidranger.NewBasicRangerEntry(*network))
		}
	}
	for _, svc := range cfg.K8SSvc {
		_, network, _ := net.ParseCIDR(svc)
		if network != nil {
			k8sSvcRanger.Insert(cidranger.NewBasicRangerEntry(*network))
		}
	}
	for _, dns := range cfg.Dns {
		_, network, _ := net.ParseCIDR(dns)
		if network != nil {
			dnsRanger.Insert(cidranger.NewBasicRangerEntry(*network))
		}
	}
	for _, host := range cfg.InternalHosts {
		_, network, _ := net.ParseCIDR(host)
		if network != nil {
			internalHostsRanger.Insert(cidranger.NewBasicRangerEntry(*network))
		}
	}

	p.k8sPodRanger = k8sPodRanger
	p.k8sSvcRanger = k8sSvcRanger
	p.dnsRanger = dnsRanger
	p.internalHostsRanger = internalHostsRanger
	p.localHostsRanger = localhostRanger
}

// LoadDefaultPolicy loads default network policy from environment variables
func LoadDefaultPolicy() NetworkPolicy {
	policy := NetworkPolicy{
		InternalHosts: []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"169.254.0.0/16",
		},
		Localhost: []string{
			"127.0.0.0/8",
		},
	}

	if podCidr := os.Getenv("NETWORK_EXPORTER_POD_CIDR"); podCidr != "" {
		policy.K8SPod = strings.Split(podCidr, ",")
	}
	if svcCidr := os.Getenv("NETWORK_EXPORTER_SVC_CIDR"); svcCidr != "" {
		policy.K8SSvc = strings.Split(svcCidr, ",")
	}
	if dnsCidr := os.Getenv("NETWORK_EXPORTER_DNS_CIDR"); dnsCidr != "" {
		policy.Dns = strings.Split(dnsCidr, ",")
	}
	if extraInternal := os.Getenv("NETWORK_EXPORTER_EXTRA_INTERNAL_CIDR"); extraInternal != "" {
		policy.InternalHosts = append(policy.InternalHosts, strings.Split(extraInternal, ",")...)
	}

	return policy
}
