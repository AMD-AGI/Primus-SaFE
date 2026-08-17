/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// liveCorefile is the Corefile shape kubespray installs, carrying the stanza this sync writes and a
// hosts patch the site maintains by hand.
const liveCorefile = `cluster.local:53 {
    errors
    cache {
        success 9984 30
        denial 9984 5
    }
    reload
    loop
    bind 169.254.25.10
    forward . 192.168.0.3 {
        force_tcp
    }
    prometheus :9253
    health 169.254.25.10:9254
}
in-addr.arpa:53 {
    errors
    cache 30
    reload
    loop
    bind 169.254.25.10
    forward . 192.168.0.3 {
        force_tcp
    }
    prometheus :9253
}
.:53 {
    hosts {
        10.9.9.1 global.example.com
        10.9.9.2 llm-api.example.com
        fallthrough
    }
    template IN A safe.example.com {
        answer "{{ .Name }} 60 IN A 10.0.0.1"
        fallthrough
    }
    errors
    cache 30
    reload
    loop
    bind 169.254.25.10
    forward . /etc/resolv.conf
    prometheus :9253
}
`

func TestRenderCorefileRoundTripsUnchanged(t *testing.T) {
	cf, err := parseCorefile(liveCorefile)
	assert.NoError(t, err)
	// A Corefile this sync has nothing to change must come back byte-identical, otherwise every
	// upgrade rewrites lines the site owns.
	assert.Equal(t, liveCorefile, renderCorefile(cf, liveCorefile))
}

func TestRootServer(t *testing.T) {
	cf, err := parseCorefile(liveCorefile)
	assert.NoError(t, err)
	root, count := rootServer(cf)
	assert.Equal(t, 1, count)
	assert.NotNil(t, root)
	assert.Equal(t, []string{".:53"}, root.DomPorts)

	// A zone written without a port is the same root zone.
	cf, err = parseCorefile(". {\n    errors\n}\n")
	assert.NoError(t, err)
	root, count = rootServer(cf)
	assert.Equal(t, 1, count)
	assert.NotNil(t, root)

	// A nested "forward . x {" is a directive, not a second root zone.
	cf, err = parseCorefile(".:53 {\n    forward . /etc/resolv.conf {\n        force_tcp\n    }\n}\n")
	assert.NoError(t, err)
	_, count = rootServer(cf)
	assert.Equal(t, 1, count)

	cf, err = parseCorefile(".:53 {\n    errors\n}\n.:53 {\n    errors\n}\n")
	assert.NoError(t, err)
	_, count = rootServer(cf)
	assert.Equal(t, 2, count)
}

func TestClusterDNSAddress(t *testing.T) {
	cf, err := parseCorefile(liveCorefile)
	assert.NoError(t, err)
	// The address comes from the zones nodelocaldns already aims at CoreDNS, not from a Service name.
	addr, ok := clusterDNSAddress(cf)
	assert.True(t, ok)
	assert.Equal(t, "192.168.0.3", addr)

	// The reverse zones still identify it when the cluster domain is customised.
	cf, err = parseCorefile("in-addr.arpa:53 {\n    forward . 10.96.0.10\n}\n.:53 {\n    errors\n}\n")
	assert.NoError(t, err)
	addr, ok = clusterDNSAddress(cf)
	assert.True(t, ok)
	assert.Equal(t, "10.96.0.10", addr)

	cf, err = parseCorefile(".:53 {\n    forward . /etc/resolv.conf\n}\n")
	assert.NoError(t, err)
	_, ok = clusterDNSAddress(cf)
	assert.False(t, ok)
}

func TestUpsertTemplateStanzaKeepsSiblingsAndPosition(t *testing.T) {
	cf, err := parseCorefile(liveCorefile)
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	upsertTemplateStanza(root, "safe.example.com", []string{"10.0.0.2", "10.0.0.3"})
	out := renderCorefile(cf, liveCorefile)

	// The hosts patch and every other directive survive.
	assert.Contains(t, out, "10.9.9.1 global.example.com")
	assert.Contains(t, out, "10.9.9.2 llm-api.example.com")
	assert.Contains(t, out, "health 169.254.25.10:9254")
	assert.Contains(t, out, "IN A 10.0.0.2")
	assert.Contains(t, out, "IN A 10.0.0.3")
	assert.NotContains(t, out, "IN A 10.0.0.1")
	// The stanza keeps its place, so the diff stays limited to the answers.
	assert.Less(t, strings.Index(out, "hosts {"), strings.Index(out, "template IN A safe.example.com"))
	assert.Equal(t, 1, strings.Count(out, "template IN A safe.example.com"))
	// The answer is quoted exactly once.
	assert.Contains(t, out, `answer "{{ .Name }} 60 IN A 10.0.0.2"`)
}

func TestUpsertTemplateStanzaDropsDuplicates(t *testing.T) {
	stanza := "    template IN A safe.example.com {\n        answer \"{{ .Name }} 60 IN A 10.9.9.9\"\n        fallthrough\n    }\n"
	cf, err := parseCorefile(".:53 {\n" + stanza + stanza + "    forward . /etc/resolv.conf\n}\n")
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	upsertTemplateStanza(root, "safe.example.com", []string{"10.0.0.1"})
	out := renderCorefile(cf, liveCorefile)
	assert.Equal(t, 1, strings.Count(out, "template IN A safe.example.com"))
	assert.NotContains(t, out, "10.9.9.9")
}

func TestRemoveTemplateStanza(t *testing.T) {
	cf, err := parseCorefile(liveCorefile)
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	removeTemplateStanza(root, "safe.example.com")
	out := renderCorefile(cf, liveCorefile)
	assert.NotContains(t, out, "template IN A safe.example.com")
	// Only the stanza this sync owns goes; the site's records stay.
	assert.Contains(t, out, "10.9.9.1 global.example.com")
	assert.Contains(t, out, "forward . /etc/resolv.conf")
}

func TestSetRootForwardKeepsSiteOptions(t *testing.T) {
	cf, err := parseCorefile(".:53 {\n    forward . /etc/resolv.conf {\n        max_concurrent 1000\n    }\n}\n")
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	setRootForward(root, "192.168.0.3", true)
	out := renderCorefile(cf, liveCorefile)
	assert.Contains(t, out, "forward . 192.168.0.3 {")
	assert.Contains(t, out, "max_concurrent 1000")
	assert.Contains(t, out, "force_tcp")
	assert.Equal(t, 1, strings.Count(out, "force_tcp"))

	// Reapplying adds nothing.
	setRootForward(root, "192.168.0.3", true)
	assert.Equal(t, out, renderCorefile(cf, liveCorefile))
}

func TestSetRootForwardReplacesMultipleUpstreams(t *testing.T) {
	cf, err := parseCorefile(".:53 {\n    forward . 10.10.10.10 10.10.10.11\n}\n")
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	setRootForward(root, "192.168.0.3", true)
	out := renderCorefile(cf, liveCorefile)
	assert.NotContains(t, out, "10.10.10.10")
	assert.Contains(t, out, "forward . 192.168.0.3 {")
}

func TestRevertRootForward(t *testing.T) {
	// A sub-block holding nothing but force_tcp was written by this sync, so it goes too.
	cf, err := parseCorefile(".:53 {\n    forward . 192.168.0.3 {\n        force_tcp\n    }\n}\n")
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	revertRootForward(root)
	out := renderCorefile(cf, liveCorefile)
	assert.Contains(t, out, "forward . /etc/resolv.conf\n")
	assert.NotContains(t, out, "force_tcp")

	// Options the site set are kept, only the address goes back.
	cf, err = parseCorefile(".:53 {\n    forward . 192.168.0.3 {\n        max_concurrent 1000\n        force_tcp\n    }\n}\n")
	assert.NoError(t, err)
	root, _ = rootServer(cf)
	revertRootForward(root)
	out = renderCorefile(cf, liveCorefile)
	assert.Contains(t, out, "forward . /etc/resolv.conf {")
	assert.Contains(t, out, "max_concurrent 1000")
}

func TestNodeLocalDNSBindIP(t *testing.T) {
	cf, err := parseCorefile(liveCorefile)
	assert.NoError(t, err)
	assert.Equal(t, "169.254.25.10", nodeLocalDNSBindIP(cf))

	cf, err = parseCorefile("cluster.local:53 {\n    bind 169.254.20.10\n}\n")
	assert.NoError(t, err)
	assert.Equal(t, "169.254.20.10", nodeLocalDNSBindIP(cf))

	cf, err = parseCorefile("cluster.local:53 {\n    kubernetes\n}\n")
	assert.NoError(t, err)
	assert.Equal(t, defaultNodeLocalDNSBindIP, nodeLocalDNSBindIP(cf))
}

func TestUnmanageableCorefileRefusesComments(t *testing.T) {
	// The lexer discards comments, so a file carrying them cannot be reproduced. Refusing is loud
	// and recoverable; rewriting it would delete the note explaining why a hosts entry exists.
	commented := `# managed by ops, do not edit without telling #infra
.:53 {
    # split-horizon workaround, see ticket OPS-1234
    hosts {
        10.9.9.1 intra-a.example.com
        fallthrough
    }
    errors
    forward . /etc/resolv.conf
}
`
	reason := unmanageableCorefileReason(commented)
	assert.Contains(t, reason, "cannot preserve")
	assert.Contains(t, reason, "#")
}

func TestUnmanageableCorefileRefusesDeeperNesting(t *testing.T) {
	// The plugin model holds two levels, so a third is dropped: braces balance, the parse succeeds
	// and the content is gone.
	nested := ".:53 {\n    forward . 10.0.0.1 {\n        tls {\n            server_name dns.example.com\n        }\n    }\n}\n"
	assert.Contains(t, unmanageableCorefileReason(nested), "cannot preserve")
}

func TestUnmanageableCorefileToleratesLayout(t *testing.T) {
	// Layout alone must never take a Corefile out of management.
	tabbed := strings.ReplaceAll(liveCorefile, "    ", "\t")
	assert.Empty(t, unmanageableCorefileReason(tabbed))
	assert.Empty(t, unmanageableCorefileReason(strings.ReplaceAll(liveCorefile, "\n", "\r\n")))
	assert.Empty(t, unmanageableCorefileReason(strings.ReplaceAll(liveCorefile, "}\n", "}\n\n")))
}

func TestClusterDNSAddressPrefersClusterDomain(t *testing.T) {
	// A site may delegate the reverse zones to an internal PTR server; the cluster domain wins.
	cf, err := parseCorefile("in-addr.arpa:53 {\n    forward . 10.1.1.1\n}\n" +
		"cluster.local:53 {\n    forward . 192.168.0.3\n}\n.:53 {\n    errors\n}\n")
	assert.NoError(t, err)
	addr, ok := clusterDNSAddress(cf)
	assert.True(t, ok)
	assert.Equal(t, "192.168.0.3", addr)
}

func TestClusterDNSAddressRefusesDisagreement(t *testing.T) {
	// Without the cluster domain to arbitrate, reverse zones aimed at different servers mean the
	// assumption does not hold for this cluster, so no address is returned.
	cf, err := parseCorefile("in-addr.arpa:53 {\n    forward . 10.1.1.1\n}\n" +
		"ip6.arpa:53 {\n    forward . 10.2.2.2\n}\n.:53 {\n    errors\n}\n")
	assert.NoError(t, err)
	_, ok := clusterDNSAddress(cf)
	assert.False(t, ok)
}

func TestRevertRootForwardDropsForceTCPAmongSiteOptions(t *testing.T) {
	// force_tcp was added by this sync, so it goes even when the site has its own options: the node
	// resolver is queried over UDP, and the revert happens when resolution is already broken.
	cf, err := parseCorefile(".:53 {\n    forward . 192.168.0.3 {\n        max_concurrent 1000\n        force_tcp\n    }\n}\n")
	assert.NoError(t, err)
	root, _ := rootServer(cf)
	revertRootForward(root)
	out := renderCorefile(cf, liveCorefile)
	assert.Contains(t, out, "forward . /etc/resolv.conf {")
	assert.Contains(t, out, "max_concurrent 1000")
	assert.NotContains(t, out, "force_tcp")
}

func TestUnmanageableCorefileReason(t *testing.T) {
	assert.Empty(t, unmanageableCorefileReason(liveCorefile))
	assert.Contains(t, unmanageableCorefileReason(liveCorefile+".:53 {\n    errors\n}\n"), "2 root zone servers")
	assert.Contains(t, unmanageableCorefileReason(".:53 {\n    errors\n"), "unbalanced")

	// A brace inside a comment is not nesting, so a commented-out block is not read as unbalanced.
	// The file is still refused, because the serialiser would drop the comment.
	commented := "# TODO: tidy this {\n" + liveCorefile
	assert.True(t, isCorefileBalanced(commented))
	assert.Contains(t, unmanageableCorefileReason(commented), "cannot preserve")

	// A brace inside a quoted answer is data.
	assert.Empty(t, unmanageableCorefileReason(
		".:53 {\n    template IN A a.b {\n        answer \"{{ .Name }} 60 IN A 10.0.0.1\"\n    }\n}\n"))
}

func TestNewRootServerUsesGivenBindIP(t *testing.T) {
	cf, err := parseCorefile("cluster.local:53 {\n    bind 169.254.20.10\n}\n")
	assert.NoError(t, err)
	cf.Servers = append(cf.Servers, newRootServer(nodeLocalDNSBindIP(cf)))
	out := renderCorefile(cf, liveCorefile)
	assert.Equal(t, 2, strings.Count(out, "bind 169.254.20.10"))
	// A created server starts on the node resolver; setRootForward owns the target afterwards.
	assert.Contains(t, out, "forward . /etc/resolv.conf")
	assert.Contains(t, out, "loop")
}

func TestSummarizeCorefileChange(t *testing.T) {
	assert.Equal(t, "+0/-0 lines", summarizeCorefileChange(liveCorefile, liveCorefile))
	assert.Contains(t, summarizeCorefileChange(liveCorefile, liveCorefile+"extra\n"), "+1/-0")
}
