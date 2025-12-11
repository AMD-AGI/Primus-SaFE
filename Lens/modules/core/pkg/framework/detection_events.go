package framework

import (
	"context"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// DetectionEventType represents the type of detection event
type DetectionEventType string

const (
	// DetectionEventTypeCompleted is fired when detection completes successfully
	DetectionEventTypeCompleted DetectionEventType = "completed"
	// DetectionEventTypeFailed is fired when detection fails
	DetectionEventTypeFailed DetectionEventType = "failed"
	// DetectionEventTypeUpdated is fired when detection is updated
	DetectionEventTypeUpdated DetectionEventType = "updated"
	// DetectionEventTypeConflict is fired when a conflict is detected
	DetectionEventTypeConflict DetectionEventType = "conflict"
)

// DetectionEvent represents an event triggered by detection operations
type DetectionEvent struct {
	Type        DetectionEventType
	WorkloadUID string
	Detection   *model.FrameworkDetection
	Error       error
}

// DetectionEventListener is the interface for listening to detection events
type DetectionEventListener interface {
	// OnDetectionEvent handles a detection event
	// Implementations should be non-blocking
	OnDetectionEvent(ctx context.Context, event *DetectionEvent) error
}

// EventDispatcher manages event listeners and dispatches events
type EventDispatcher struct {
	mu        sync.RWMutex
	listeners []DetectionEventListener
}

// NewEventDispatcher creates a new event dispatcher
func NewEventDispatcher() *EventDispatcher {
	return &EventDispatcher{
		listeners: make([]DetectionEventListener, 0),
	}
}

// RegisterListener registers a new event listener
func (d *EventDispatcher) RegisterListener(listener DetectionEventListener) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.listeners = append(d.listeners, listener)
	log.Infof("Registered detection event listener (total: %d)", len(d.listeners))
}

// UnregisterListener removes an event listener
func (d *EventDispatcher) UnregisterListener(listener DetectionEventListener) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i, l := range d.listeners {
		if l == listener {
			d.listeners = append(d.listeners[:i], d.listeners[i+1:]...)
			log.Infof("Unregistered detection event listener (remaining: %d)", len(d.listeners))
			return
		}
	}
}

// Dispatch dispatches an event to all registered listeners
// This method is non-blocking and runs each listener in a separate goroutine
func (d *EventDispatcher) Dispatch(ctx context.Context, event *DetectionEvent) {
	d.mu.RLock()
	listeners := make([]DetectionEventListener, len(d.listeners))
	copy(listeners, d.listeners)
	d.mu.RUnlock()

	if len(listeners) == 0 {
		log.Debugf("No listeners registered for detection event: type=%s, workload=%s",
			event.Type, event.WorkloadUID)
		return
	}

	log.Debugf("Dispatching detection event: type=%s, workload=%s to %d listeners",
		event.Type, event.WorkloadUID, len(listeners))

	// Dispatch to all listeners concurrently
	for _, listener := range listeners {
		// Create a copy for the goroutine
		l := listener

		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("Listener panicked: %v", r)
				}
			}()

			// Use a new context to avoid transaction context issues
			// The parent context might be bound to a DB transaction that will be
			// committed/rolled back before this goroutine executes
			listenerCtx := context.Background()

			if err := l.OnDetectionEvent(listenerCtx, event); err != nil {
				log.Errorf("Listener error for event type=%s, workload=%s: %v",
					event.Type, event.WorkloadUID, err)
			} else {
				log.Debugf("Listener handled event successfully: type=%s, workload=%s",
					event.Type, event.WorkloadUID)
			}
		}()
	}
}

// GetListenerCount returns the number of registered listeners
func (d *EventDispatcher) GetListenerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.listeners)
}
