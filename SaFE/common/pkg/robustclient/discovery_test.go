/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func readyCluster(name string, annotations map[string]string) *v1.Cluster {
	c := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: annotations}}
	c.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	return c
}

// A sync registers every ready cluster at its annotated endpoint or the
// in-cluster default, skips one not ready, and drops one whose CR is gone.
func TestSyncOnce(t *testing.T) {
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	k8s := ctrlfake.NewClientBuilder().WithScheme(s).WithObjects(
		readyCluster("local", nil),
		readyCluster("remote", map[string]string{annotationRobustEndpoint: "http://10.0.0.1:32626"}),
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "pending"}},
	).Build()
	rc := NewClient(ClientConfig{Timeout: time.Second})
	rc.RegisterCluster("gone", "http://old")
	d := NewDiscovery(k8s, rc, time.Minute)
	resolves := true
	d.lookupHost = func(_ context.Context, host string) ([]string, error) {
		if resolves {
			return []string{"192.168.0.10"}, nil
		}
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}

	d.syncOnce(context.Background())
	assert.Equal(t, robustAPIEndpoint, rc.ForCluster("local").BaseURL())
	assert.Equal(t, "http://10.0.0.1:32626", rc.ForCluster("remote").BaseURL())
	assert.Nil(t, rc.ForCluster("pending"))
	assert.Nil(t, rc.ForCluster("gone"))

	// robust-api goes away: the local cluster moves to robust-analyzer.
	resolves = false
	d.syncOnce(context.Background())
	assert.Equal(t, robustAnalyzerEndpoint, rc.ForCluster("local").BaseURL())
	assert.Equal(t, "http://10.0.0.1:32626", rc.ForCluster("remote").BaseURL())
}
