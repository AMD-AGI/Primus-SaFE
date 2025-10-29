/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package channel

// Tomb is used to control the lifecycle of a goroutine.
type Tomb struct {
	stop chan struct{}
	done chan struct{}
}

// NewTomb creates a new tomb.
func NewTomb() *Tomb {
	return &Tomb{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Stop is used to stop the goroutine outside.
func (t *Tomb) Stop() {
	close(t.stop)
	<-t.done
}

// Stopping is used by the goroutine to tell whether it should stop.
func (t *Tomb) Stopping() <-chan struct{} {
	return t.stop
}

// Done is used by the goroutine to inform that it has stopped.
func (t *Tomb) Done() {
	close(t.done)
}

func (t *Tomb) IsStopped() bool {
	return IsChannelClosed(t.stop)
}
