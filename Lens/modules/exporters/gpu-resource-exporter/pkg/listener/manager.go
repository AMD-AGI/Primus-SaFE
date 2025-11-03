package listener

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
)

type Manager struct {
	listeners  map[string]*Listener
	client     *clientsets.K8SClientSet
	l          sync.RWMutex
	ctx        context.Context
	cancelFunc context.CancelFunc
}

var manager *Manager

func InitManager(ctx context.Context) error {
	if manager == nil {
		manager = newManager(ctx)
	}
	err := manager.RecoverListener(ctx)
	if err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				manager.garbageCollect()
			case <-manager.ctx.Done():
				log.Infof("Manager context cancelled, stopping garbage collection")
				return
			}
		}
	}()
	return nil
}

func GetManager() *Manager {
	return manager
}

func newManager(ctx context.Context) *Manager {
	managerCtx, cancel := context.WithCancel(ctx)
	return &Manager{
		listeners:  make(map[string]*Listener),
		client:     clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet,
		ctx:        managerCtx,
		cancelFunc: cancel,
	}
}

// Shutdown gracefully shuts down the manager and all listeners
func (m *Manager) Shutdown() {
	log.Infof("Shutting down listener manager")
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	// Wait for all listeners to finish
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Warnf("Timeout waiting for listeners to finish, %d listeners still running", len(m.listeners))
			return
		case <-ticker.C:
			m.l.RLock()
			allEnded := true
			for _, listener := range m.listeners {
				if !listener.IsEnd() {
					allEnded = false
					break
				}
			}
			m.l.RUnlock()

			if allEnded {
				log.Infof("All listeners finished")
				return
			}
		}
	}
}

func (m *Manager) RegisterListener(apiVersion, kind, namespace, name, uid string) error {
	m.l.Lock()
	defer m.l.Unlock()
	if _, exists := m.listeners[uid]; exists {
		log.Infof("Listener for %s/%s (%s) already exists", namespace, name, uid)
		return nil
	}
	log.Infof("Registering listener for %s/%s (%s)", namespace, name, uid)
	listener, err := NewListener(apiVersion, kind, namespace, name, uid, m.client)
	if err != nil {
		return err
	}
	m.listeners[uid] = listener
	log.Infof("Registered listener for %s/%s (%s)", namespace, name, uid)
	// Use manager's context instead of Background() for unified lifecycle management
	listener.Start(m.ctx)
	log.Infof("Listener for %s/%s (%s) is now running", namespace, name, uid)
	return nil
}

func (m *Manager) RecoverListener(ctx context.Context) error {
	workloads, err := database.GetFacade().GetWorkload().GetWorkloadNotEnd(ctx)
	if err != nil {
		return err
	}
	for _, wl := range workloads {
		err := m.RegisterListener(wl.GroupVersion, wl.Kind, wl.Namespace, wl.Name, wl.UID)
		if err != nil {
			log.Errorf("Failed to register listener for %s/%s: %v", wl.Namespace, wl.Name, err)
		}
	}
	return nil
}

func (m *Manager) garbageCollect() {
	m.l.Lock()
	defer m.l.Unlock()
	log.Infof("Garbage collecting %d resources", len(m.listeners))
	deleted := 0
	for uid, listener := range m.listeners {
		if listener.IsEnd() {
			deleted += 1
			delete(m.listeners, uid)
		}
	}
	log.Infof("Garbage collected %d resources, %d remaining", deleted, len(m.listeners))
}
