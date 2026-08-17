/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"fmt"
	"strings"

	"github.com/coredns/corefile-migration/migration/corefile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

const (
	// nodeLocalDNSNamespace and nodeLocalDNSConfigMap locate the Corefile this sync manages.
	nodeLocalDNSNamespace = "kube-system"
	nodeLocalDNSConfigMap = "nodelocaldns"
	corefileKey           = "Corefile"
	// nodeLocalDNSForwardAnnotation records that this sync, rather than the site, pointed the root
	// server's forward at the cluster DNS. Address equality cannot stand in for that: a site is free
	// to aim its own root forward at the same CoreDNS, and undoing it would break resolution the
	// site set up deliberately. Keeping the record on the object also survives a manager restart.
	nodeLocalDNSForwardAnnotation = v1.PrimusSafePrefix + "nodelocaldns.forward"
	nodeLocalDNSForwardClusterDNS = "cluster-dns"
	// defaultDNSUpstream forwards to whatever the node itself resolves against.
	defaultDNSUpstream = "/etc/resolv.conf"
	// defaultNodeLocalDNSBindIP is the link-local address nodelocaldns listens on. kubespray exposes
	// it as nodelocaldns_ip, so it is only a fallback for a Corefile that declares no bind address.
	defaultNodeLocalDNSBindIP = "169.254.25.10"
	forwardPlugin             = "forward"
	templatePlugin            = "template"
	bindPlugin                = "bind"
	forceTCPOption            = "force_tcp"
	answerOption              = "answer"
	fallthroughOption         = "fallthrough"
)

// rootZones are the zone specifications CoreDNS serves the root zone from. A zone written without a
// port defaults to 53, so both forms name the same server.
var rootZones = map[string]bool{".": true, ".:53": true}

// clusterDomainZone is the default cluster domain, and clusterZoneHints name the zones nodelocaldns
// forwards to its cluster's CoreDNS. The reverse zones have fixed names, so they still identify that
// address when the cluster domain is customised.
const clusterDomainZone = "cluster.local"

var clusterZoneHints = []string{clusterDomainZone, "in-addr.arpa", "ip6.arpa"}

// parseCorefile reads a Corefile into its server and plugin structure.
func parseCorefile(text string) (*corefile.Corefile, error) {
	cf, err := corefile.New(text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Corefile: %w", err)
	}
	return cf, nil
}

// renderCorefile serialises a Corefile the way it was written. The servers are concatenated rather
// than joined, because the joining separator would add a blank line between them, and the trailing
// newline follows the original, so a Corefile this sync has nothing to change is not rewritten.
func renderCorefile(cf *corefile.Corefile, original string) string {
	var sb strings.Builder
	for _, server := range cf.Servers {
		if len(server.Plugins) == 0 {
			// Server.ToString drops the braces of an empty block, which CoreDNS cannot parse.
			sb.WriteString(strings.Join(server.DomPorts, " ") + " {\n}\n")
			continue
		}
		sb.WriteString(server.ToString())
	}
	out := sb.String()
	if !strings.HasSuffix(original, "\n") {
		return strings.TrimRight(out, "\n")
	}
	return out
}

// rootServer returns the server holding the root zone, and how many declare it. CoreDNS serves the
// root zone from exactly one server and rejects the whole file when it is declared twice.
func rootServer(cf *corefile.Corefile) (*corefile.Server, int) {
	var found *corefile.Server
	var count int
	for _, server := range cf.Servers {
		for _, zone := range server.DomPorts {
			if !rootZones[zone] {
				continue
			}
			count++
			if found == nil {
				found = server
			}
			break
		}
	}
	return found, count
}

// clusterDNSAddress returns the address the cluster zones already forward to. nodelocaldns is
// configured with it at install time, so it names the cluster's own CoreDNS without assuming a
// Service name, which differs between installers. The cluster domain wins over the reverse zones,
// which a site may legitimately delegate to an internal PTR server; when the zones disagree the
// assumption this reads on does not hold for that cluster and no address is returned.
func clusterDNSAddress(cf *corefile.Corefile) (string, bool) {
	seen := map[string]string{}
	for _, server := range cf.Servers {
		hint, ok := clusterZoneHint(server)
		if !ok {
			continue
		}
		plugin := findForward(server)
		if plugin == nil || len(plugin.Args) < 2 {
			continue
		}
		if _, dup := seen[hint]; !dup {
			seen[hint] = plugin.Args[1]
		}
	}
	if addr, ok := seen[clusterDomainZone]; ok {
		return addr, true
	}
	var chosen string
	for _, addr := range seen {
		if chosen == "" {
			chosen = addr
			continue
		}
		if addr != chosen {
			// The reverse zones point at different servers, so neither can be assumed to be the
			// cluster's own CoreDNS.
			return "", false
		}
	}
	return chosen, chosen != ""
}

// clusterZoneHint returns which of the zones aimed at the cluster's CoreDNS the server handles.
func clusterZoneHint(server *corefile.Server) (string, bool) {
	for _, zone := range server.DomPorts {
		name := zone
		if colon := strings.LastIndex(name, ":"); colon > 0 {
			name = name[:colon]
		}
		for _, hint := range clusterZoneHints {
			if name == hint {
				return hint, true
			}
		}
	}
	return "", false
}

// findForward returns the server's "forward ." directive.
func findForward(server *corefile.Server) *corefile.Plugin {
	for _, plugin := range server.Plugins {
		if plugin.Name == forwardPlugin && len(plugin.Args) >= 1 && plugin.Args[0] == "." {
			return plugin
		}
	}
	return nil
}

// setRootForward points the root server at upstream, adding force_tcp when asked. Options the site
// set are kept: forwarding to a cluster Service over UDP is the conntrack path nodelocaldns avoids,
// but the rest of the directive is not this sync's to rewrite.
func setRootForward(server *corefile.Server, upstream string, forceTCP bool) {
	plugin := findForward(server)
	if plugin == nil {
		plugin = &corefile.Plugin{Name: forwardPlugin}
		server.Plugins = append(server.Plugins, plugin)
	}
	plugin.Args = []string{".", upstream}
	if forceTCP && !hasOption(plugin, forceTCPOption) {
		plugin.Options = append(plugin.Options, &corefile.Option{Name: forceTCPOption})
	}
}

// revertRootForward points the root server back at the node resolver, dropping the force_tcp this
// sync added. The node resolver is queried over UDP, and an upstream that refuses TCP/53 is common
// enough that leaving a transport nobody chose on the path taken when resolution is already broken
// would be the worst place for it. Other options belong to the site and are kept.
func revertRootForward(server *corefile.Server) {
	plugin := findForward(server)
	if plugin == nil {
		return
	}
	plugin.Args = []string{".", defaultDNSUpstream}
	kept := make([]*corefile.Option, 0, len(plugin.Options))
	for _, option := range plugin.Options {
		if option.Name != forceTCPOption {
			kept = append(kept, option)
		}
	}
	if len(kept) == 0 {
		plugin.Options = nil
		return
	}
	plugin.Options = kept
}

// hasOption reports whether the plugin already sets the named option.
func hasOption(plugin *corefile.Plugin, name string) bool {
	for _, option := range plugin.Options {
		if option.Name == name {
			return true
		}
	}
	return false
}

// upsertTemplateStanza points the template answering dnsName at the given addresses, keeping its
// position so an unchanged Corefile is not reordered. Duplicates are dropped rather than left
// answering with an address this sync no longer considers healthy.
func upsertTemplateStanza(server *corefile.Server, dnsName string, ips []string) {
	desired := templateStanza(dnsName, ips)
	kept := make([]*corefile.Plugin, 0, len(server.Plugins))
	var placed bool
	for _, plugin := range server.Plugins {
		if !isTemplateFor(plugin, dnsName) {
			kept = append(kept, plugin)
			continue
		}
		if !placed {
			kept = append(kept, desired)
			placed = true
		}
	}
	if !placed {
		// A new stanza goes first, matching the shape this sync has always written.
		kept = append([]*corefile.Plugin{desired}, kept...)
	}
	server.Plugins = kept
}

// removeTemplateStanza drops every template answering dnsName. A site routing the root server to
// CoreDNS resolves the name there, and CoreDNS orders template ahead of hosts, so a stanza left
// behind would silently override the record that site maintains.
func removeTemplateStanza(server *corefile.Server, dnsName string) {
	kept := make([]*corefile.Plugin, 0, len(server.Plugins))
	for _, plugin := range server.Plugins {
		if !isTemplateFor(plugin, dnsName) {
			kept = append(kept, plugin)
		}
	}
	server.Plugins = kept
}

// isTemplateFor reports whether the plugin is the A-record template for dnsName.
func isTemplateFor(plugin *corefile.Plugin, dnsName string) bool {
	return plugin.Name == templatePlugin && len(plugin.Args) == 3 &&
		plugin.Args[0] == "IN" && plugin.Args[1] == "A" && plugin.Args[2] == dnsName
}

// templateStanza builds the template plugin resolving dnsName to every given address. One A record
// per control plane lets a resolver fall over when the first stops answering. The answer is stored
// unquoted because the serialiser quotes arguments containing whitespace.
func templateStanza(dnsName string, ips []string) *corefile.Plugin {
	plugin := &corefile.Plugin{
		Name: templatePlugin,
		Args: []string{"IN", "A", dnsName},
	}
	for _, ip := range ips {
		plugin.Options = append(plugin.Options, &corefile.Option{
			Name: answerOption,
			Args: []string{fmt.Sprintf("{{ .Name }} 60 IN A %s", ip)},
		})
	}
	plugin.Options = append(plugin.Options, &corefile.Option{Name: fallthroughOption})
	return plugin
}

// newRootServer builds a root server for a Corefile that has none, on the node resolver so that
// setRootForward owns the target afterwards.
func newRootServer(bindIP string) *corefile.Server {
	return &corefile.Server{
		DomPorts: []string{".:53"},
		Plugins: []*corefile.Plugin{
			{Name: "errors"},
			{Name: "cache", Args: []string{"30"}},
			{Name: "reload"},
			{Name: "loop"},
			{Name: bindPlugin, Args: []string{bindIP}},
			{Name: forwardPlugin, Args: []string{".", defaultDNSUpstream}},
			{Name: "prometheus", Args: []string{":9253"}},
		},
	}
}

// nodeLocalDNSBindIP returns the address the Corefile already binds to, so a site that moved
// nodelocaldns off the default link-local address keeps its own. Any server will do: nodelocaldns
// binds all of them to the same address.
func nodeLocalDNSBindIP(cf *corefile.Corefile) string {
	for _, server := range cf.Servers {
		for _, plugin := range server.Plugins {
			if plugin.Name == bindPlugin && len(plugin.Args) == 1 {
				return plugin.Args[0]
			}
		}
	}
	return defaultNodeLocalDNSBindIP
}

// unmanageableCorefileReason describes why a Corefile cannot be managed, or returns empty when it
// can. Both conditions produce a file CoreDNS refuses to load, and neither can be repaired without
// knowing which parts the site means to keep.
func unmanageableCorefileReason(text string) string {
	// The parser folds an unclosed block into its predecessor rather than failing, so the brace
	// balance is checked here. This is the only place that reads the Corefile as text.
	if !isCorefileBalanced(text) {
		return "has unbalanced braces"
	}
	cf, err := parseCorefile(text)
	if err != nil {
		return "cannot be parsed"
	}
	if _, count := rootServer(cf); count > 1 {
		return fmt.Sprintf("declares %d root zone servers", count)
	}
	// A Corefile this serialiser cannot reproduce holds something it would drop: a comment, which
	// the lexer discards, or a block nested deeper than the plugin options the parser models.
	// Reformatting is acceptable, losing a line is not, so the comparison ignores layout.
	if lost := droppedLines(text, renderCorefile(cf, text)); len(lost) > 0 {
		return fmt.Sprintf("uses syntax this sync cannot preserve (%q)", lost[0])
	}
	return ""
}

// droppedLines returns the content lines of before that after no longer holds, ignoring indentation
// and blank lines so that layout alone never takes a Corefile out of management.
func droppedLines(before, after string) []string {
	remaining := make(map[string]int, 32)
	for _, line := range contentLines(after) {
		remaining[line]++
	}
	var lost []string
	for _, line := range contentLines(before) {
		if remaining[line] > 0 {
			remaining[line]--
			continue
		}
		lost = append(lost, line)
	}
	return lost
}

// contentLines splits a Corefile into its non-blank lines with surrounding whitespace removed.
func contentLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// isCorefileBalanced reports whether every block in the Corefile is closed. Braces inside a comment
// or a quoted string are data, not nesting.
func isCorefileBalanced(text string) bool {
	var depth int
	inQuote, inComment := false, false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\n' {
			// A Corefile token does not span lines, so both states end with the line.
			inQuote, inComment = false, false
			continue
		}
		if inComment {
			continue
		}
		switch {
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '#':
			inComment = true
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

// summarizeCorefileChange reports the line delta of a Corefile rewrite so an unexpected loss of
// site configuration is visible in the log without dumping the whole file.
func summarizeCorefileChange(before, after string) string {
	return fmt.Sprintf("+%d/-%d lines", len(droppedLines(after, before)), len(droppedLines(before, after)))
}
