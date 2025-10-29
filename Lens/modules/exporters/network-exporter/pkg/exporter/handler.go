package exporter

import (
	"context"
	"net"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/goroutineUtil"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/bpf/tcpconn"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/bpf/tcpflow"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/model"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/policy"
	"github.com/AMD-AGI/primus-lens/network-exporter/pkg/util"
)

var (
	singleton *Handler
)

func InitNetHandler(conf *config.Config) (*Handler, error) {
	var err error
	if singleton == nil {
		singleton, err = newHandler(conf)
		if err != nil {
			return nil, err
		}
	}
	return singleton, nil
}

type Handler struct {
	nodeName       string
	tcpConn        *tcpconn.BpfTcpConn
	tcpFlow        *tcpflow.BpfTcpFlow
	localIpAddress map[string]struct{}
	lg             logger.Logger
	localListen    map[int]struct{}
	ranger         *policyRanger
	tcpTmpCache    *tcpTmpCache
	metricsCache   *metricsCache
	metrics        *networkMetricsSet
}

func newTcpTmpCache() *tcpTmpCache {
	return &tcpTmpCache{
		tcpFlowCache: make(map[model.TcpFlowCacheKey]*model.TcpFlowDataValue),
		tcpConnCache: make(map[model.TcpFlowCacheKey]*model.TcpFlowDataValue),
	}
}

type tcpTmpCache struct {
	tcpFlowCache map[model.TcpFlowCacheKey]*model.TcpFlowDataValue
	tcpConnCache map[model.TcpFlowCacheKey]*model.TcpFlowDataValue
}

func newMetricsCache() *metricsCache {
	return &metricsCache{
		tcpEgressFlow:  util.NewCache[model.TcpEgressMetricValue](nil, 10*time.Minute),
		tcpIngressFlow: util.NewCache[model.TcpIngressMetricValue](nil, 10*time.Minute),
		k8sFlow:        util.NewCache[float64](nil, 0),
		dnsFlow:        0,
	}
}

type metricsCache struct {
	tcpEgressFlow  *util.Cache[model.TcpEgressMetricValue]
	tcpIngressFlow *util.Cache[model.TcpIngressMetricValue]
	k8sFlow        *util.Cache[float64]
	dnsFlow        float64
}

func newHandler(conf *config.Config) (*Handler, error) {
	h := &Handler{
		localIpAddress: make(map[string]struct{}),
		lg:             log.GlobalLogger().WithField("module", "netflow"),
		localListen:    map[int]struct{}{},
		tcpTmpCache:    newTcpTmpCache(),
		metricsCache:   newMetricsCache(),
		metrics:        newNetworkMetricsSet(),
	}
	h.setPolicy(policy.GetDefaultPolicy())
	return h, nil
}

func (n *Handler) Init(ctx context.Context, conf *config.Config) error {
	var err error
	if err = n.loadLocalIpAddress(); err != nil {
		return err
	}
	n.tcpConn, err = tcpconn.NewBpfTcpConn()
	if err != nil {
		return err
	}
	n.tcpConn.InitChan(409600)
	n.tcpFlow, err = tcpflow.NewBpfTcpFlow()
	if err != nil {
		return err
	}
	n.tcpFlow.InitChan(409600)
	n.tcpConn.Start()
	n.tcpFlow.Start()
	err = n.loadListeningPort()
	if err != nil {
		return err
	}
	n.register()
	goroutineUtil.RunGoroutineWithLog(func() {
		n.runLoadListenPortProcess(ctx, conf.Netflow.GetScanPortListenInterval())
	})
	goroutineUtil.RunGoroutineWithLog(func() {
		n.syncTcpConn(ctx)
	})
	goroutineUtil.RunGoroutineWithLog(func() {
		n.syncTcpFlow(ctx)
	})
	goroutineUtil.RunGoroutineWithLog(func() {
		n.doFlushTcpFlow(ctx)
	})
	goroutineUtil.RunGoroutineWithLog(n.flushNetworkMetrics)
	return nil
}

func (n *Handler) runLoadListenPortProcess(ctx context.Context, interval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := n.loadListeningPort()
			if err != nil {
				n.lg.Errorf("load listen port failed %s", err)
			}
		}
		time.Sleep(interval)
	}
}

func (n *Handler) loadListeningPort() error {
	ports, err := n.getAllListingPort()
	if err != nil {
		return err
	}
	log.GlobalLogger().WithField("ports", ports).Debugf("load listen port %d", len(ports))
	listenPort := map[int]struct{}{}
	for _, port := range ports {
		listenPort[port] = struct{}{}
	}
	n.localListen = listenPort
	return nil
}

func (n *Handler) getAllListingPort() ([]int, error) {
	tcpPorts, err := util.TcpListenWithCustomPath("/host-proc/net/tcp")
	if err != nil {
		return nil, err
	}
	n.lg.Debugf("tcp port count %d", len(tcpPorts))
	results := []int{}
	for _, port := range tcpPorts {
		if port.LocalAddr == nil {
			n.lg.Warningf("local port is nil")
			continue
		}
		results = append(results, int(port.LocalAddr.Port))
	}
	return results, nil
}

func (n *Handler) loadLocalIpAddress() error {
	// load local ip address
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	n.lg.Debugf("local ip address count %d", len(interfaces))
	// 遍历每个网络接口
	for _, iface := range interfaces {
		n.lg.Debugf("interface %s", iface.Name)
		// 获取接口的地址
		addrs, err := iface.Addrs()
		if err != nil {
			n.lg.Errorf("get interface %s address failed %s", iface.Name, err)
			continue
		}
		n.lg.Debugf("interface %s address count %d", iface.Name, len(addrs))
		// 遍历每个地址
		for _, addr := range addrs {
			switch ip := addr.(type) {
			case *net.IPNet:
				n.localIpAddress[ip.IP.String()] = struct{}{}
			case *net.IPAddr:
				n.localIpAddress[ip.IP.String()] = struct{}{}
			}
		}
	}
	for ip := range n.localIpAddress {
		log.GlobalLogger().Debugf("local ip address %s", ip)
	}
	return nil
}

func (n *Handler) getDirection(saddr, daddr string, sport, dport uint16) (localAddr, remoteAddr string, localPort, remotePort int, typ int, direction string) {
	saddrLocal := false
	sPortListen := false
	daddrLocal := false
	dPortListen := false
	if _, ok := n.localIpAddress[saddr]; ok {
		saddrLocal = true
		if _, ok := n.localListen[int(sport)]; ok {
			sPortListen = true
		}
	}
	if _, ok := n.localIpAddress[daddr]; ok {
		daddrLocal = true
		if _, ok := n.localListen[int(dport)]; ok {
			dPortListen = true
		}
	}
	if saddrLocal && daddrLocal && !sPortListen && !dPortListen {
		typ = -1
		return
	}
	if sPortListen || dPortListen {
		typ = model.FlowTypeIngress
		if sPortListen {
			direction = model.DirectionOutbound
			localAddr = saddr
			remoteAddr = daddr
			localPort = int(sport)
			remotePort = int(dport)
		} else {
			direction = model.DirectionInbound
			localAddr = daddr
			remoteAddr = saddr
			localPort = int(dport)
			remotePort = int(sport)
		}
	} else {
		typ = model.FlowTypeEgress
		if saddrLocal {
			direction = model.DirectionOutbound
			localAddr = saddr
			remoteAddr = daddr
			localPort = int(sport)
			remotePort = int(dport)
		} else {
			direction = model.DirectionInbound
			localAddr = daddr
			remoteAddr = saddr
			localPort = int(dport)
			remotePort = int(sport)
		}
	}
	return
}
