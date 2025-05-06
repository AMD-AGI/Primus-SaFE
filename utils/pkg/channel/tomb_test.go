/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package channel

import (
	"reflect"
	"testing"
)

func TestTomb(t *testing.T) {
	tomb := NewTomb()
	var workflow []string
	expected := []string{"stop", "stopping", "stopped"}
	go func() {
		defer tomb.Done()
		<-tomb.Stopping()
		workflow = append(workflow, "stopping")
	}()
	workflow = append(workflow, "stop")
	tomb.Stop()
	workflow = append(workflow, "stopped")
	if !reflect.DeepEqual(workflow, expected) {
		t.Errorf("expected workflow %v, got %v", expected, workflow)
	}
}
