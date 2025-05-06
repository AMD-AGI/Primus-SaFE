package exporters

import (
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
)

type Exporter interface {
	Handle(message *types.MonitorMessage) error
	Name() string
}

type ExporterManager struct {
	queue     *types.MonitorQueue
	tomb      *channel.Tomb
	exporters []Exporter
	isExited  bool
}

func NewExporterManager(queue *types.MonitorQueue, node *node.Node) *ExporterManager {
	m := &ExporterManager{
		queue: queue,
		tomb:  channel.NewTomb(),
	}
	m.Register(&K8sExporter{node: node})
	return m
}

func (m *ExporterManager) Register(e Exporter) {
	m.exporters = append(m.exporters, e)
}

func (m *ExporterManager) Start() {
	go func() {
		m.isExited = false
		defer func() {
			m.tomb.Done()
		}()
		for {
			select {
			case <-m.tomb.Stopping():
				return
			default:
				if shutdown := m.Dispatch(); shutdown {
					break
				}
			}
		}
	}()
}

func (m *ExporterManager) Stop() {
	if m.isExited {
		return
	}
	m.tomb.Stop()
	m.isExited = true
}

func (m *ExporterManager) IsExited() bool {
	return m.isExited
}

func (m *ExporterManager) Dispatch() bool {
	message, shutdown := (*m.queue).Get()
	if shutdown {
		return true
	}
	defer (*m.queue).Done(message)

	for i := range m.exporters {
		if err := m.exporters[i].Handle(message); err != nil {
			klog.ErrorS(err, "failed to handle message",
				"exporter", m.exporters[i].Name(), "message", message)
		}
	}
	(*m.queue).Forget(message)
	return false
}
