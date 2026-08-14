/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"fmt"
	"strings"
)

const (
	// nodeLocalDNSNamespace and nodeLocalDNSConfigMap locate the Corefile this sync manages.
	nodeLocalDNSNamespace = "kube-system"
	nodeLocalDNSConfigMap = "nodelocaldns"
	// clusterDNSService is the Service fronting CoreDNS in every cluster. The name predates
	// CoreDNS and is kept for compatibility.
	clusterDNSService = "kube-dns"
	// dnsServerBlockPrefix opens the server block this sync creates when a Corefile has no root one.
	dnsServerBlockPrefix = ".:53 {"
	// dnsStanzaIndent is the indentation CoreDNS Corefiles use for directives inside a block.
	dnsStanzaIndent = "    "
	// defaultNodeLocalDNSBindIP is the link-local address nodelocaldns listens on. kubespray exposes
	// it as nodelocaldns_ip, so it is only a fallback for a Corefile that declares no bind address.
	defaultNodeLocalDNSBindIP = "169.254.25.10"
	// defaultDNSUpstream forwards to whatever the node itself resolves against.
	defaultDNSUpstream = "/etc/resolv.conf"
	// templateStanzaMarker prefixes the stanza this sync owns inside the root block.
	templateStanzaMarker = "template IN A "
)

// rootZoneTokens are the zone specifications CoreDNS treats as the root zone on port 53. A zone
// written without a port defaults to 53, so both forms open the same block.
var rootZoneTokens = map[string]bool{".": true, ".:53": true}

// renderTemplateStanza renders the CoreDNS template stanza resolving dnsName to every given
// address. One A record per control plane lets a resolver fall over when the first stops answering.
// The result is a whole number of lines so it can be spliced into a block without reindenting it.
func renderTemplateStanza(dnsName string, ips []string) string {
	var sb strings.Builder
	sb.WriteString(dnsStanzaIndent + templateStanzaMarker + dnsName + " {\n")
	for _, ip := range ips {
		fmt.Fprintf(&sb, "%s%sanswer \"{{ .Name }} 60 IN A %s\"\n", dnsStanzaIndent, dnsStanzaIndent, ip)
	}
	sb.WriteString(dnsStanzaIndent + dnsStanzaIndent + "fallthrough\n")
	sb.WriteString(dnsStanzaIndent + "}\n")
	return sb.String()
}

// renderForwardDirective renders the root block's forward directive. force_tcp goes into a
// sub-block because a cluster Service upstream must be reached over TCP to avoid the UDP conntrack
// races nodelocaldns is deployed to prevent.
func renderForwardDirective(upstream string, forceTCP bool) string {
	if !forceTCP {
		return dnsStanzaIndent + "forward . " + upstream + "\n"
	}
	return dnsStanzaIndent + "forward . " + upstream + " {\n" +
		dnsStanzaIndent + dnsStanzaIndent + "force_tcp\n" +
		dnsStanzaIndent + "}\n"
}

// renderRootServerBlock builds a complete root server block, used only for Corefiles that have
// none. Sites that already have one keep their own directives.
func renderRootServerBlock(dnsName string, ips []string, bindIP string) string {
	return fmt.Sprintf("%s\n%s%serrors\n%scache 30\n%sreload\n%sloop\n%sbind %s\n%s%sprometheus :9253\n}\n",
		dnsServerBlockPrefix, renderTemplateStanza(dnsName, ips),
		dnsStanzaIndent, dnsStanzaIndent, dnsStanzaIndent, dnsStanzaIndent,
		dnsStanzaIndent, bindIP,
		renderForwardDirective(defaultDNSUpstream, false), dnsStanzaIndent)
}

// upsertTemplateStanza points the template stanza for dnsName at the given addresses. Only that
// stanza is rewritten, so hosts entries and any other directive sharing the block survive. A root
// block is created only when the Corefile has none, keeping that block unique. The forward
// directive of a created block is left at its default; applyForwardUpstream owns that line.
func upsertTemplateStanza(corefile, dnsName string, ips []string) string {
	stanza := renderTemplateStanza(dnsName, ips)
	if start, end, ok := findTemplateStanza(corefile, dnsName); ok {
		// Retarget the first stanza and drop any duplicate, which would otherwise keep answering
		// with addresses this sync no longer considers healthy.
		return stripTemplateStanzas(corefile[:start]+stanza+corefile[end:], dnsName, start+len(stanza))
	}
	if _, blockEnd, ok := findRootServerBlock(corefile); ok {
		insertAt := lineStartIndex(corefile, blockEnd-1)
		return corefile[:insertAt] + stanza + corefile[insertAt:]
	}
	// A Corefile we cannot delimit is left untouched rather than gaining a second root block.
	if hasUndelimitedBlock(corefile, dnsName) {
		return corefile
	}
	return strings.TrimRight(corefile, " \t\n") + "\n" +
		renderRootServerBlock(dnsName, ips, nodeLocalDNSBindIP(corefile))
}

// stripTemplateStanzas removes every template stanza for dnsName found at or after from.
func stripTemplateStanzas(corefile, dnsName string, from int) string {
	for from < len(corefile) {
		start, end, ok := findTemplateStanzaFrom(corefile, dnsName, from)
		if !ok {
			return corefile
		}
		corefile = corefile[:start] + corefile[end:]
		from = start
	}
	return corefile
}

// removeTemplateStanza drops every template stanza for dnsName. A site that routes the root block to
// CoreDNS resolves the system host there, and CoreDNS orders template ahead of hosts, so leaving the
// stanza behind would silently override the records that site maintains.
func removeTemplateStanza(corefile, dnsName string) string {
	return stripTemplateStanzas(corefile, dnsName, 0)
}

// applyForwardUpstream points the root block's "forward ." directive at upstream so a site can
// route everything it does not resolve locally to CoreDNS instead of the node resolver. An empty
// upstream leaves the directive exactly as the site configured it.
func applyForwardUpstream(corefile, upstream string, forceTCP bool) string {
	if upstream == "" {
		return corefile
	}
	start, end, ok := findRootForward(corefile)
	if !ok {
		blockStart, blockEnd, found := findRootServerBlock(corefile)
		if !found {
			return corefile
		}
		_ = blockStart
		closingLine := lineStartIndex(corefile, blockEnd-1)
		return corefile[:closingLine] + renderForwardDirective(upstream, forceTCP) + corefile[closingLine:]
	}

	options := forwardOptions(corefile[start:end])
	if len(options) == 0 {
		return corefile[:start] + renderForwardDirective(upstream, forceTCP) + corefile[end:]
	}
	// The site owns these options, so they are kept. force_tcp is added when missing because
	// forwarding to a cluster Service over UDP is the conntrack path nodelocaldns exists to avoid.
	if forceTCP && !containsOption(options, "force_tcp") {
		options = append(options, "force_tcp")
	}
	return corefile[:start] + renderForwardBlock(upstream, options) + corefile[end:]
}

// revertRootForward points the root block's forward directive back at the node resolver, used when
// the site turns forwarding off. A sub-block holding nothing but force_tcp was written by this sync,
// so it goes with the address; anything else is the site's and is kept.
func revertRootForward(corefile string) string {
	start, end, ok := findRootForward(corefile)
	if !ok {
		return corefile
	}
	options := forwardOptions(corefile[start:end])
	if len(options) == 0 || (len(options) == 1 && options[0] == "force_tcp") {
		return corefile[:start] + renderForwardDirective(defaultDNSUpstream, false) + corefile[end:]
	}
	return corefile[:start] + renderForwardBlock(defaultDNSUpstream, options) + corefile[end:]
}

// rootForwardTarget returns the address the root block forwards to.
func rootForwardTarget(corefile string) (string, bool) {
	start, end, ok := findRootForward(corefile)
	if !ok {
		return "", false
	}
	fields := strings.Fields(codeSpan(corefile[start:lineEndIndex(corefile, start)]))
	_ = end
	if len(fields) < 3 {
		return "", false
	}
	return fields[2], true
}

// findRootForward returns the byte range of the root block's "forward ." directive, including the
// options sub-block when it has one, spanning whole lines.
func findRootForward(corefile string) (int, int, bool) {
	blockStart, blockEnd, ok := findRootServerBlock(corefile)
	if !ok {
		return 0, 0, false
	}
	closingLine := lineStartIndex(corefile, blockEnd-1)
	for offset := advancePastNewline(corefile, blockStart); offset < closingLine; {
		lineEnd := lineEndIndex(corefile, offset)
		fields := strings.Fields(codeSpan(corefile[offset:lineEnd]))
		if len(fields) >= 2 && fields[0] == "forward" && fields[1] == "." {
			if fields[len(fields)-1] != "{" {
				return offset, advancePastNewline(corefile, offset), true
			}
			subEnd := matchingBraceEnd(corefile, offset)
			if subEnd < 0 {
				return offset, advancePastNewline(corefile, offset), true
			}
			return offset, advancePastNewline(corefile, subEnd-1), true
		}
		offset = advancePastNewline(corefile, offset)
	}
	return 0, 0, false
}

// forwardOptions returns the directives inside a forward sub-block, empty when it has none.
func forwardOptions(directive string) []string {
	var options []string
	lines := strings.Split(strings.TrimRight(directive, "\n"), "\n")
	if len(lines) < 2 {
		return nil
	}
	for _, line := range lines[1 : len(lines)-1] {
		if option := strings.TrimSpace(codeSpan(line)); option != "" {
			options = append(options, option)
		}
	}
	return options
}

// containsOption reports whether options already sets the named directive.
func containsOption(options []string, name string) bool {
	for _, option := range options {
		if option == name || strings.HasPrefix(option, name+" ") {
			return true
		}
	}
	return false
}

// renderForwardBlock renders a forward directive carrying the given options.
func renderForwardBlock(upstream string, options []string) string {
	var sb strings.Builder
	sb.WriteString(dnsStanzaIndent + "forward . " + upstream + " {\n")
	for _, option := range options {
		sb.WriteString(dnsStanzaIndent + dnsStanzaIndent + option + "\n")
	}
	sb.WriteString(dnsStanzaIndent + "}\n")
	return sb.String()
}

// nodeLocalDNSBindIP returns the address the Corefile already binds to, so a site that moved
// nodelocaldns off the default link-local address keeps its own. Any block will do: nodelocaldns
// binds every one of them to the same address.
func nodeLocalDNSBindIP(corefile string) string {
	for offset := 0; offset < len(corefile); offset = advancePastNewline(corefile, offset) {
		fields := strings.Fields(corefile[offset:lineEndIndex(corefile, offset)])
		if len(fields) == 2 && fields[0] == "bind" {
			return fields[1]
		}
	}
	return defaultNodeLocalDNSBindIP
}

// findRootServerBlock returns the byte range of the root server block, header line included. A
// block whose braces are unbalanced reports no match, leaving a Corefile we do not fully understand
// intact.
func findRootServerBlock(corefile string) (int, int, bool) {
	for offset, depth := 0, 0; offset < len(corefile); offset = advancePastNewline(corefile, offset) {
		line := corefile[offset:lineEndIndex(corefile, offset)]
		if depth == 0 && isRootZoneHeader(line) {
			end := matchingBraceEnd(corefile, offset)
			if end < 0 {
				return 0, 0, false
			}
			return offset, end, true
		}
		depth += braceDelta(line)
	}
	return 0, 0, false
}

// countRootServerBlocks reports how many root server blocks the Corefile declares. CoreDNS serves
// the root zone from exactly one block and rejects the whole file when it is defined twice.
func countRootServerBlocks(corefile string) int {
	var blocks int
	for offset, depth := 0, 0; offset < len(corefile); offset = advancePastNewline(corefile, offset) {
		line := corefile[offset:lineEndIndex(corefile, offset)]
		if depth == 0 && isRootZoneHeader(line) {
			blocks++
		}
		depth += braceDelta(line)
	}
	return blocks
}

// isRootZoneHeader reports whether line opens a server block whose zone list contains the root zone.
// Callers must only offer lines at brace depth zero: a directive such as "forward . x {" carries the
// same tokens and is a zone list only at the top level.
func isRootZoneHeader(line string) bool {
	fields := strings.Fields(codeSpan(line))
	if len(fields) < 2 || fields[len(fields)-1] != "{" {
		return false
	}
	for _, zone := range fields[:len(fields)-1] {
		if rootZoneTokens[zone] {
			return true
		}
	}
	return false
}

// braceDelta reports how much a line changes the block nesting depth. CoreDNS template actions such
// as "{{ .Name }}" are balanced, so they cancel out.
func braceDelta(line string) int {
	code := codeSpan(line)
	return strings.Count(code, "{") - strings.Count(code, "}")
}

// codeSpan returns the part of line CoreDNS parses, dropping a trailing comment. A brace inside a
// comment must not shift the nesting depth, otherwise every later block header is misread.
func codeSpan(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return line[:i]
			}
		}
	}
	return line
}

// unmanageableCorefileReason describes why a Corefile cannot be managed, or returns empty when it
// can. Both conditions produce a file CoreDNS refuses to load and neither can be repaired without
// knowing which parts the site means to keep.
func unmanageableCorefileReason(corefile string) string {
	if !isCorefileBalanced(corefile) {
		return "has unbalanced braces"
	}
	if blocks := countRootServerBlocks(corefile); blocks > 1 {
		return fmt.Sprintf("declares %d root zone blocks", blocks)
	}
	return ""
}

// isCorefileBalanced reports whether every block in the Corefile is closed. An unbalanced file is one
// CoreDNS refuses to load, and one this sync cannot reason about, so it is left alone.
func isCorefileBalanced(corefile string) bool {
	var depth int
	for offset := 0; offset < len(corefile); offset = advancePastNewline(corefile, offset) {
		depth += braceDelta(corefile[offset:lineEndIndex(corefile, offset)])
		if depth < 0 {
			return false
		}
	}
	return depth == 0
}

// findTemplateStanza returns the byte range of the first template stanza for dnsName.
func findTemplateStanza(corefile, dnsName string) (int, int, bool) {
	return findTemplateStanzaFrom(corefile, dnsName, 0)
}

// findTemplateStanzaFrom returns the byte range of the first template stanza for dnsName at or after
// from, spanning whole lines so removing it leaves no partial line behind.
func findTemplateStanzaFrom(corefile, dnsName string, from int) (int, int, bool) {
	marker := templateStanzaMarker + dnsName
	for searchFrom := from; searchFrom < len(corefile); {
		rel := strings.Index(corefile[searchFrom:], marker)
		if rel < 0 {
			return 0, 0, false
		}
		pos := searchFrom + rel
		after := pos + len(marker)
		// Reject a longer name that merely starts with dnsName.
		if after < len(corefile) && !isStanzaBoundary(corefile[after]) {
			searchFrom = after
			continue
		}
		end, ok := templateStanzaEnd(corefile, after)
		if !ok {
			return 0, 0, false
		}
		return lineStartIndex(corefile, pos), end, true
	}
	return 0, 0, false
}

// templateStanzaEnd returns the index just past the line closing the stanza whose marker ends at
// after, reporting false when its braces are unbalanced.
func templateStanzaEnd(corefile string, after int) (int, bool) {
	lineEnd := lineEndIndex(corefile, after)
	brace := strings.IndexByte(corefile[after:lineEnd], '{')
	if brace < 0 {
		return advancePastNewline(corefile, after), true
	}
	end := matchingBraceEnd(corefile, after+brace)
	if end < 0 {
		return 0, false
	}
	return advancePastNewline(corefile, end), true
}

// hasUndelimitedBlock reports whether the Corefile already declares a root zone or a template stanza
// for dnsName that could not be delimited. The scan deliberately ignores nesting depth: a line this
// sync cannot parse would shift the depth-tracked scan and hide a root zone that is really there,
// and appending a second root block to such a file is the outcome this sync exists to prevent.
func hasUndelimitedBlock(corefile, dnsName string) bool {
	if strings.Contains(corefile, templateStanzaMarker+dnsName) {
		return true
	}
	for offset := 0; offset < len(corefile); offset = advancePastNewline(corefile, offset) {
		if isRootZoneHeader(corefile[offset:lineEndIndex(corefile, offset)]) {
			return true
		}
	}
	return false
}

// isStanzaBoundary reports whether c can legally follow a domain name in a Corefile directive.
func isStanzaBoundary(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '{'
}

// matchingBraceEnd returns the index just past the brace closing the block opened at or after
// start, or -1 when the braces are unbalanced. Braces inside a comment or a quoted string are data.
func matchingBraceEnd(corefile string, start int) int {
	depth := 0
	inQuote, inComment := false, false
	for i := start; i < len(corefile); i++ {
		c := corefile[i]
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
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// summarizeCorefileChange reports the line delta of a Corefile rewrite so an unexpected loss of
// site configuration is visible in the log without dumping the whole file.
func summarizeCorefileChange(before, after string) string {
	kept := make(map[string]int, 32)
	for _, line := range strings.Split(before, "\n") {
		kept[strings.TrimSpace(line)]++
	}
	var added int
	for _, line := range strings.Split(after, "\n") {
		trimmed := strings.TrimSpace(line)
		if kept[trimmed] > 0 {
			kept[trimmed]--
			continue
		}
		added++
	}
	var removed int
	for _, count := range kept {
		removed += count
	}
	return fmt.Sprintf("+%d/-%d lines", added, removed)
}

// lineStartIndex returns the index of the first byte on the line containing pos.
func lineStartIndex(s string, pos int) int {
	if nl := strings.LastIndexByte(s[:pos], '\n'); nl >= 0 {
		return nl + 1
	}
	return 0
}

// lineEndIndex returns the index of the newline ending the line containing pos, or len(s).
func lineEndIndex(s string, pos int) int {
	if nl := strings.IndexByte(s[pos:], '\n'); nl >= 0 {
		return pos + nl
	}
	return len(s)
}

// advancePastNewline returns the index of the first byte after the line containing pos.
func advancePastNewline(s string, pos int) int {
	end := lineEndIndex(s, pos)
	if end < len(s) {
		return end + 1
	}
	return len(s)
}
