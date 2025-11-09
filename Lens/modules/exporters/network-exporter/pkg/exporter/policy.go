package exporter

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/network-exporter/pkg/policy"
	"github.com/yl2chen/cidranger"
	"net"
)

const (
	IPSourceK8sPod                = "k8sPod"
	IPSourceK8sSvc                = "k8sSvc"
	IPSourceDNS                   = "dns"
	IPSourceAbnormalFlowBlackList = "abnormalFlowBlackList"
	IPSourceAbnormalFlowWhiteList = "abnormalFlowWhiteList"
	IPSourceDocker                = "docker"
	IPSourceInternalHosts         = "internalHosts"
	IPSourceExternalHosts         = "externalHosts"
	IPSourceUnknown               = "unknown"
	IPSourceError                 = "error"
	IPSourceLocalhost             = "localhost"
)

type policyRanger struct {
	localHostsRanger            cidranger.Ranger
	k8sPodRanger                cidranger.Ranger
	k8sSvcRanger                cidranger.Ranger
	dnsRanger                   cidranger.Ranger
	abnormalFlowBlackListRanger cidranger.Ranger
	abnormalFlowWhiteListRanger cidranger.Ranger
	dockerRanger                cidranger.Ranger
	internalHostsRanger         cidranger.Ranger
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

func (p *policyRanger) matchAbnormal(ip string) (string, bool, error) {
	netIP := net.ParseIP(ip)
	if netIP == nil {
		return "", false, nil
	}
	if ok, err := p.abnormalFlowBlackListRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if ok {
		return IPSourceAbnormalFlowBlackList, true, nil
	}
	if ok, err := p.abnormalFlowWhiteListRanger.Contains(netIP); err != nil {
		return "", false, err
	} else if !ok {
		return IPSourceAbnormalFlowWhiteList, true, nil
	}
	return IPSourceUnknown, false, nil
}

func (p *policyRanger) setDockerRanger(ipRange []string) {
	dockerRanger := cidranger.NewPCTrieRanger()
	for _, ip := range ipRange {
		_, network, _ := net.ParseCIDR(ip)
		dockerRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}
	p.dockerRanger = dockerRanger
}

func (p *policyRanger) loadFromConfig(cfg policy.NetworkPolicy) {
	k8sPodRanger := cidranger.NewPCTrieRanger()
	k8sSvcRanger := cidranger.NewPCTrieRanger()
	dnsRanger := cidranger.NewPCTrieRanger()
	abnormalFlowBlackListRanger := cidranger.NewPCTrieRanger()
	abnormalFlowWhiteListRanger := cidranger.NewPCTrieRanger()
	internalHostsRanger := cidranger.NewPCTrieRanger()
	localhostRanger := cidranger.NewPCTrieRanger()
	for _, s := range cfg.Localhost {
		_, network, _ := net.ParseCIDR(s)
		localhostRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}
	for _, pod := range cfg.K8SPod {
		_, network, _ := net.ParseCIDR(pod)
		k8sPodRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}
	for _, svc := range cfg.K8SSvc {
		_, network, _ := net.ParseCIDR(svc)
		k8sSvcRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}

	for _, dns := range cfg.Dns {
		_, network, _ := net.ParseCIDR(dns)
		dnsRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}

	for _, black := range cfg.AbnormalBlackList {
		_, network, _ := net.ParseCIDR(black)
		abnormalFlowBlackListRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}

	for _, white := range cfg.AbnormalWhiteList {
		_, network, _ := net.ParseCIDR(white)
		abnormalFlowWhiteListRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}
	for _, host := range cfg.InternalHosts {
		_, network, _ := net.ParseCIDR(host)
		internalHostsRanger.Insert(cidranger.NewBasicRangerEntry(*network))
	}
	p.k8sPodRanger = k8sPodRanger
	p.k8sSvcRanger = k8sSvcRanger
	p.dnsRanger = dnsRanger
	p.abnormalFlowBlackListRanger = abnormalFlowBlackListRanger
	p.abnormalFlowWhiteListRanger = abnormalFlowWhiteListRanger
	p.internalHostsRanger = internalHostsRanger
	p.localHostsRanger = localhostRanger
}

func (h *Handler) setPolicy(policy policy.NetworkPolicy) {
	if h.ranger == nil {
		h.ranger = &policyRanger{}
	}
	h.ranger.loadFromConfig(policy)
}
