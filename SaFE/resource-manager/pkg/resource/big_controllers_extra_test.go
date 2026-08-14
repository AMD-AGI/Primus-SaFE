/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func newClusterReconciler(t *testing.T) *ClusterReconciler {
	t.Helper()
	scheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	return &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
}

// ---- model_controller pure functions ----

func TestIsS3ImportModel(t *testing.T) {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{v1.ModelS3ImportLabel: v1.TrueStr}}}
	assert.True(t, isS3ImportModel(m))
	assert.False(t, isS3ImportModel(&v1.Model{}))
	assert.False(t, isS3ImportModel(nil))
}

func TestBuildHTTPURLFromS3URI(t *testing.T) {
	m := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{v1.ModelS3SourceEndpointAnn: "minio:9000"}},
		Spec:       v1.ModelSpec{Source: v1.ModelSource{URL: "s3://bucket/prefix"}},
	}
	url, err := buildHTTPURLFromS3URI(m)
	assert.NoError(t, err)
	assert.Equal(t, "https://minio:9000/bucket/prefix/", url)

	// Not an s3 URI.
	_, err = buildHTTPURLFromS3URI(&v1.Model{Spec: v1.ModelSpec{Source: v1.ModelSource{URL: "http://x"}}})
	assert.Error(t, err)
}

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"a", "b"}, "a"))
	assert.False(t, containsString([]string{"a"}, "z"))
}

func TestConstructDownloadJobErrors(t *testing.T) {
	r := newMockModelReconciler(nil)
	// Empty source URL -> error.
	_, err := r.constructDownloadJob(&v1.Model{})
	assert.Error(t, err)
}

// ---- cluster_controller pure functions ----

func TestEndpointSubsetEqual(t *testing.T) {
	a := corev1.EndpointSubset{
		Addresses: []corev1.EndpointAddress{{IP: "1.1.1.1"}},
		Ports:     []corev1.EndpointPort{{Port: 80}},
	}
	b := a.DeepCopy()
	assert.True(t, endpointSubsetEqual(a, *b))

	diff := corev1.EndpointSubset{Addresses: []corev1.EndpointAddress{{IP: "2.2.2.2"}}, Ports: []corev1.EndpointPort{{Port: 80}}}
	assert.False(t, endpointSubsetEqual(a, diff))
}

func TestEndpointsSubsetsChanged(t *testing.T) {
	a := []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "1.1.1.1"}}}}
	assert.False(t, endpointsSubsetsChanged(a, a))
	assert.True(t, endpointsSubsetsChanged(a, nil))
}

func TestIsClusterSourceEndpoints(t *testing.T) {
	r := newClusterReconciler(t)
	ep := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	assert.True(t, r.isClusterSourceEndpoints(ep))
	// Forward EP -> false.
	fwd := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1-forward", Namespace: common.PrimusSafeNamespace}}
	assert.False(t, r.isClusterSourceEndpoints(fwd))
	// Wrong namespace -> false.
	other := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "other"}}
	assert.False(t, r.isClusterSourceEndpoints(other))
}

// corefileWithHosts mirrors a site Corefile whose hosts patch shares the root block with the
// template stanza the controller owns.
const corefileWithHosts = `cluster.local:53 {
    kubernetes
}
.:53 {
    hosts {
        10.9.9.1 intra-a.example.com
        10.9.9.2 intra-b.example.com
        fallthrough
    }
    template IN A foo.local {
        answer "{{ .Name }} 60 IN A 10.0.0.1"
        fallthrough
    }
    errors
    health 169.254.25.10:9254
    bind 169.254.25.10
    forward . /etc/resolv.conf
}
`

func TestRenderTemplateStanza(t *testing.T) {
	stanza := renderTemplateStanza("foo.local", []string{"10.0.0.1", "10.0.0.3"})
	assert.Contains(t, stanza, "foo.local")
	// Every healthy control plane gets its own A record so resolvers can fall over.
	assert.Contains(t, stanza, "IN A 10.0.0.1")
	assert.Contains(t, stanza, "IN A 10.0.0.3")
	// Splicing the stanza into a block must not disturb the surrounding lines.
	assert.True(t, strings.HasSuffix(stanza, "\n"))
	assert.NotContains(t, stanza, dnsServerBlockPrefix)
}

func TestUpsertTemplateStanzaKeepsSiblingDirectives(t *testing.T) {
	out := upsertTemplateStanza(corefileWithHosts, "foo.local", []string{"10.0.0.3"})

	// The hosts patch and every other directive sharing the block survive the rewrite.
	for _, want := range []string{
		"hosts {", "10.9.9.1 intra-a.example.com", "10.9.9.2 intra-b.example.com",
		"health 169.254.25.10:9254", "cluster.local:53 {",
	} {
		assert.Contains(t, out, want)
	}
	assert.Equal(t, 1, strings.Count(out, dnsServerBlockPrefix))
	assert.Contains(t, out, "IN A 10.0.0.3")
	assert.NotContains(t, out, "IN A 10.0.0.1")
}

func TestUpsertTemplateStanzaIsIdempotent(t *testing.T) {
	once := upsertTemplateStanza(corefileWithHosts, "foo.local", []string{"10.0.0.3"})
	assert.Equal(t, once, upsertTemplateStanza(once, "foo.local", []string{"10.0.0.3"}))
}

func TestUpsertTemplateStanzaReusesExistingRootBlock(t *testing.T) {
	// No template stanza yet: the stanza joins the existing block instead of opening a second one.
	withoutTemplate := "cluster.local:53 {\n    kubernetes\n}\n.:53 {\n    hosts {\n        10.9.9.1 intra-a.example.com\n    }\n    forward . /etc/resolv.conf\n}\n"
	out := upsertTemplateStanza(withoutTemplate, "foo.local", []string{"10.0.0.1"})
	assert.Equal(t, 1, strings.Count(out, dnsServerBlockPrefix))
	assert.Contains(t, out, "10.9.9.1 intra-a.example.com")
	assert.Contains(t, out, "template IN A foo.local {")
}

func TestUpsertTemplateStanzaCreatesRootBlockWhenAbsent(t *testing.T) {
	out := upsertTemplateStanza("cluster.local:53 {\n    kubernetes\n}\n", "foo.local", []string{"10.0.0.1"})
	assert.Equal(t, 1, strings.Count(out, dnsServerBlockPrefix))
	assert.Contains(t, out, "forward . /etc/resolv.conf")
}

func TestUpsertTemplateStanzaIgnoresFQDNZoneHeader(t *testing.T) {
	// A zone written with a trailing dot contains the ".:53 {" substring but is not the root zone.
	corefile := "global.example.com.:53 {\n    forward . 192.168.0.10\n}\n.:53 {\n    forward . /etc/resolv.conf\n}\n"
	out := upsertTemplateStanza(corefile, "foo.local", []string{"10.0.0.1"})
	assert.Contains(t, out, "global.example.com.:53 {")
	assert.Contains(t, out, "forward . 192.168.0.10")
	// The stanza belongs to the real root block, not the FQDN zone.
	assert.Less(t, strings.Index(out, "forward . 192.168.0.10"), strings.Index(out, "template IN A foo.local"))
}

func TestUpsertTemplateStanzaKeepsUnbalancedCorefile(t *testing.T) {
	// A block we cannot delimit is preserved instead of being mangled.
	broken := ".:53 {\n    template IN A foo.local {\n        answer \"x\""
	assert.Equal(t, broken, upsertTemplateStanza(broken, "foo.local", []string{"10.0.0.1"}))
}

func TestApplyForwardUpstream(t *testing.T) {
	// An empty upstream leaves the site's own directive alone.
	assert.Equal(t, corefileWithHosts, applyForwardUpstream(corefileWithHosts, "", false))

	out := applyForwardUpstream(corefileWithHosts, "192.168.0.10", false)
	assert.Contains(t, out, "forward . 192.168.0.10")
	assert.NotContains(t, out, "forward . /etc/resolv.conf")
	// Only the root block is retargeted; other zones keep forwarding where the site sent them.
	assert.Contains(t, out, "cluster.local:53 {")
	assert.Contains(t, out, "10.9.9.1 intra-a.example.com")
}

func TestApplyForwardUpstreamKeepsForwardSubBlock(t *testing.T) {
	corefile := ".:53 {\n    forward . /etc/resolv.conf {\n        force_tcp\n    }\n}\n"
	out := applyForwardUpstream(corefile, "192.168.0.10", false)
	assert.Contains(t, out, "forward . 192.168.0.10 {")
	assert.Contains(t, out, "force_tcp")
}

func TestApplyForwardUpstreamAddsMissingDirective(t *testing.T) {
	corefile := ".:53 {\n    errors\n}\n"
	out := applyForwardUpstream(corefile, "192.168.0.10", false)
	assert.Contains(t, out, "forward . 192.168.0.10")
	assert.Contains(t, out, "errors")
}

func TestApplyForwardUpstreamForceTCP(t *testing.T) {
	// A cluster Service upstream is reached over TCP, matching how the cluster.local zones forward.
	out := applyForwardUpstream(corefileWithHosts, "192.168.0.10", true)
	assert.Contains(t, out, "forward . 192.168.0.10 {")
	assert.Contains(t, out, "force_tcp")
	assert.NotContains(t, out, "forward . /etc/resolv.conf")
	assert.Contains(t, out, "10.9.9.1 intra-a.example.com")
	// The rewritten block must stay balanced so CoreDNS can still parse it.
	assert.Equal(t, strings.Count(out, "{"), strings.Count(out, "}"))
	// Reapplying must not nest another sub-block.
	assert.Equal(t, out, applyForwardUpstream(out, "192.168.0.10", true))
}

func TestCountRootServerBlocks(t *testing.T) {
	assert.Equal(t, 1, countRootServerBlocks(corefileWithHosts))
	assert.Equal(t, 2, countRootServerBlocks(corefileWithHosts+".:53 {\n    errors\n}\n"))
	assert.Equal(t, 0, countRootServerBlocks("cluster.local:53 {\n    kubernetes\n}\n"))
	// A zone written with a trailing dot is not the root zone.
	assert.Equal(t, 0, countRootServerBlocks("a.example.com.:53 {\n    errors\n}\n"))
	// A zone written without a port defaults to 53, so it is the same root zone.
	assert.Equal(t, 1, countRootServerBlocks(". {\n    errors\n}\n"))
	assert.Equal(t, 2, countRootServerBlocks(". {\n    errors\n}\n.:53 {\n    errors\n}\n"))
	// "forward . x {" carries the same tokens as a root zone list but is nested inside a block.
	assert.Equal(t, 1, countRootServerBlocks(".:53 {\n    forward . /etc/resolv.conf {\n        force_tcp\n    }\n}\n"))
}

func TestUpsertTemplateStanzaReusesPortlessRootBlock(t *testing.T) {
	// A bare "." zone is the root zone, so the stanza must join that block rather than open a
	// second root block CoreDNS would refuse to load.
	corefile := "cluster.local:53 {\n    kubernetes\n}\n" +
		". {\n    hosts {\n        10.9.9.1 intra-a.example.com\n        fallthrough\n    }\n" +
		"    forward . /etc/resolv.conf\n}\n"
	out := upsertTemplateStanza(corefile, "foo.local", []string{"10.0.0.1"})
	assert.Equal(t, 1, countRootServerBlocks(out))
	assert.NotContains(t, out, dnsServerBlockPrefix)
	assert.Contains(t, out, "10.9.9.1 intra-a.example.com")
	assert.Contains(t, out, "template IN A foo.local {")
}

func TestUpsertTemplateStanzaDropsDuplicateStanzas(t *testing.T) {
	// Two stanzas for one name: the survivor is retargeted and the stale duplicate is removed
	// rather than left answering with an address that is no longer healthy.
	stanza := "    template IN A foo.local {\n        answer \"{{ .Name }} 60 IN A 10.9.9.9\"\n        fallthrough\n    }\n"
	corefile := dnsServerBlockPrefix + "\n" + stanza + stanza + "    forward . /etc/resolv.conf\n}\n"

	out := upsertTemplateStanza(corefile, "foo.local", []string{"10.0.0.1"})
	assert.Equal(t, 1, strings.Count(out, "template IN A foo.local {"))
	assert.NotContains(t, out, "10.9.9.9")
	assert.Contains(t, out, "IN A 10.0.0.1")
	assert.Equal(t, strings.Count(out, "{"), strings.Count(out, "}"))
}

func TestRemoveTemplateStanza(t *testing.T) {
	out := removeTemplateStanza(corefileWithHosts, "foo.local")
	assert.NotContains(t, out, "template IN A foo.local")
	// Only the stanza this sync owns is dropped; the site's own directives stay.
	assert.Contains(t, out, "10.9.9.1 intra-a.example.com")
	assert.Contains(t, out, "health 169.254.25.10:9254")
	assert.Equal(t, 1, countRootServerBlocks(out))
	assert.Equal(t, strings.Count(out, "{"), strings.Count(out, "}"))
	// Removing again changes nothing.
	assert.Equal(t, out, removeTemplateStanza(out, "foo.local"))
}

func TestCommentedBraceDoesNotHideRootBlock(t *testing.T) {
	// A partially commented-out block leaves a brace CoreDNS ignores. Counting it would shift the
	// depth-tracked scan and hide every later block header.
	corefile := "# TODO: tidy this {\n" +
		"cluster.local:53 {\n    kubernetes\n}\n" +
		".:53 {\n    hosts {\n        10.9.9.1 intra-a.example.com\n        fallthrough\n    }\n" +
		"    forward . /etc/resolv.conf\n}\n"

	assert.Equal(t, 1, countRootServerBlocks(corefile))
	assert.True(t, isCorefileBalanced(corefile))
	out := upsertTemplateStanza(corefile, "foo.local", []string{"10.0.0.1"})
	assert.Equal(t, 1, countRootServerBlocks(out), "must not append a second root block")
	assert.Contains(t, out, "10.9.9.1 intra-a.example.com")
	assert.Contains(t, out, "template IN A foo.local {")
}

func TestUnbalancedCorefileIsNeverAppendedTo(t *testing.T) {
	// Braces this sync cannot balance must not lead to a second root block, whatever hid the first.
	corefile := "cluster.local:53 {\n    kubernetes\n" +
		".:53 {\n    forward . /etc/resolv.conf\n}\n"

	assert.False(t, isCorefileBalanced(corefile))
	assert.NotEmpty(t, unmanageableCorefileReason(corefile))
	assert.Equal(t, corefile, upsertTemplateStanza(corefile, "foo.local", []string{"10.0.0.1"}))
}

func TestUnmanageableCorefileReason(t *testing.T) {
	assert.Empty(t, unmanageableCorefileReason(corefileWithHosts))
	assert.Contains(t, unmanageableCorefileReason(corefileWithHosts+".:53 {\n}\n"), "2 root zone blocks")
	assert.Contains(t, unmanageableCorefileReason(".:53 {\n"), "unbalanced")
	// A brace inside a quoted string is data, not nesting.
	assert.Empty(t, unmanageableCorefileReason(".:53 {\n    template IN A a.b {\n        answer \"{{ .Name }} 60 IN A 10.0.0.1\"\n    }\n}\n"))
}

func TestApplyForwardUpstreamAddsForceTCPToExistingSubBlock(t *testing.T) {
	// The site owns max_concurrent, but forwarding to a cluster Service still has to use TCP.
	corefile := ".:53 {\n    forward . /etc/resolv.conf {\n        max_concurrent 1000\n    }\n}\n"
	out := applyForwardUpstream(corefile, "192.168.0.10", true)
	assert.Contains(t, out, "forward . 192.168.0.10 {")
	assert.Contains(t, out, "max_concurrent 1000")
	assert.Contains(t, out, "force_tcp")
	assert.Equal(t, strings.Count(out, "{"), strings.Count(out, "}"))
	// Reapplying must not add force_tcp twice.
	assert.Equal(t, out, applyForwardUpstream(out, "192.168.0.10", true))
	assert.Equal(t, 1, strings.Count(out, "force_tcp"))
}

func TestRootForwardTarget(t *testing.T) {
	target, ok := rootForwardTarget(corefileWithHosts)
	assert.True(t, ok)
	assert.Equal(t, defaultDNSUpstream, target)

	target, ok = rootForwardTarget(".:53 {\n    forward . 192.168.0.10 {\n        force_tcp\n    }\n}\n")
	assert.True(t, ok)
	assert.Equal(t, "192.168.0.10", target)

	_, ok = rootForwardTarget(".:53 {\n    errors\n}\n")
	assert.False(t, ok)
}

func TestRevertRootForward(t *testing.T) {
	// A sub-block holding nothing but force_tcp was written by this sync, so it goes too.
	ours := ".:53 {\n    hosts {\n        10.9.9.1 keep.example.com\n    }\n" +
		"    forward . 192.168.0.10 {\n        force_tcp\n    }\n}\n"
	out := revertRootForward(ours)
	assert.Contains(t, out, "forward . "+defaultDNSUpstream+"\n")
	assert.NotContains(t, out, "force_tcp")
	assert.Contains(t, out, "10.9.9.1 keep.example.com")
	assert.Equal(t, strings.Count(out, "{"), strings.Count(out, "}"))

	// Options the site set are kept, only the address goes back.
	theirs := ".:53 {\n    forward . 192.168.0.10 {\n        max_concurrent 1000\n        force_tcp\n    }\n}\n"
	out = revertRootForward(theirs)
	assert.Contains(t, out, "forward . "+defaultDNSUpstream+" {")
	assert.Contains(t, out, "max_concurrent 1000")
	assert.Equal(t, strings.Count(out, "{"), strings.Count(out, "}"))
}

func TestNodeLocalDNSBindIP(t *testing.T) {
	assert.Equal(t, "169.254.25.10", nodeLocalDNSBindIP(corefileWithHosts))
	// A site that moved nodelocaldns off the default address keeps its own.
	assert.Equal(t, "169.254.20.10", nodeLocalDNSBindIP("cluster.local:53 {\n    bind 169.254.20.10\n}\n"))
	assert.Equal(t, defaultNodeLocalDNSBindIP, nodeLocalDNSBindIP("cluster.local:53 {\n    kubernetes\n}\n"))
}

func TestRenderRootServerBlockUsesCorefileBindIP(t *testing.T) {
	out := upsertTemplateStanza("cluster.local:53 {\n    bind 169.254.20.10\n}\n", "foo.local", []string{"10.0.0.1"})
	assert.Contains(t, out, "bind 169.254.20.10")
	assert.NotContains(t, out, "bind "+defaultNodeLocalDNSBindIP)
}

func TestRenderRootServerBlock(t *testing.T) {
	block := renderRootServerBlock("foo.local", []string{"10.0.0.1"}, defaultNodeLocalDNSBindIP)
	// A created block starts on the node resolver; applyForwardUpstream owns that line afterwards.
	assert.Contains(t, block, "forward . "+defaultDNSUpstream)
	// loop guards a Corefile that forwards into the cluster against a resolution cycle.
	assert.Contains(t, block, "loop")
	assert.Equal(t, strings.Count(block, "{"), strings.Count(block, "}"))
}

func TestGenerateForwardName(t *testing.T) {
	assert.Equal(t, "c1-forward", generateForwardName("c1"))
}

func TestGenAllPriorityClass(t *testing.T) {
	classes := genAllPriorityClass("c1")
	assert.Len(t, classes, 3)
}

func TestGetControlPlaneIPsNoCluster(t *testing.T) {
	r := newClusterReconciler(t)
	_, err := r.getControlPlaneIPs(context.Background())
	assert.Error(t, err)
}

// ---- cluster_contoller_plane pure functions ----

func TestGetComponentName(t *testing.T) {
	assert.Equal(t, "comp", getComponentName("comp.suffix"))
	assert.Equal(t, "comp", getComponentName("comp"))
}

func TestShouldFetchKubeConfig(t *testing.T) {
	r := newClusterReconciler(t)
	// Created phase, no data -> true.
	c := &v1.Cluster{}
	c.Status.ControlPlaneStatus.Phase = v1.CreatedPhase
	assert.True(t, r.shouldFetchKubeConfig(c))

	// Ready phase -> false.
	c2 := &v1.Cluster{}
	c2.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	assert.False(t, r.shouldFetchKubeConfig(c2))
}

func TestAddOwnerReferences(t *testing.T) {
	r := newClusterReconciler(t)
	pod := &corev1.Pod{}
	hosts := &HostTemplateContent{Controllers: []*v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
	}}
	r.addOwnerReferences(pod, hosts)
	assert.Len(t, pod.OwnerReferences, 1)
}

func TestGenerateSSHSecretExisting(t *testing.T) {
	scheme, _ := genMockScheme()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: common.PrimusSafeNamespace}}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	r := &ClusterReconciler{ClusterBaseReconciler: &ClusterBaseReconciler{Client: cl}}
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	// Secret already exists -> no error, no creation.
	assert.NoError(t, r.generateSSHSecret(context.Background(), cluster))
}
