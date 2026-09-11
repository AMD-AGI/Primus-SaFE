/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"net"
	"syscall"
	"testing"

	testifyassert "github.com/stretchr/testify/assert"
)

// TestIsTemporaryReadsTheErrorAcceptActuallyBuilds pins the ordering inside
// isTemporary. Accept wraps every failure in a *net.OpError, which is itself a
// net.Error, so testing for net.Error before the errnos would match all of these
// and answer Timeout() - false - turning a pod that briefly ran out of file
// descriptors into a forward that is over.
func TestIsTemporaryReadsTheErrorAcceptActuallyBuilds(t *testing.T) {
	for _, errno := range []syscall.Errno{
		syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS,
		syscall.ENOMEM, syscall.ECONNABORTED, syscall.EINTR,
	} {
		err := &net.OpError{Op: "accept", Net: "tcp", Err: errno}
		testifyassert.Truef(t, isTemporary(err), "accept should be retried after %v", errno)
	}

	// A closed listener is how this process is stopped, and a genuine failure is
	// not worth spinning on.
	testifyassert.False(t, isTemporary(net.ErrClosed))
	testifyassert.False(t, isTemporary(&net.OpError{Op: "accept", Err: net.ErrClosed}))
	testifyassert.False(t, isTemporary(&net.OpError{Op: "accept", Err: syscall.EINVAL}))
}

func TestOriginOfNamesAnUnknownPeer(t *testing.T) {
	addr, port := originOf(&net.TCPAddr{IP: net.ParseIP("10.0.0.9"), Port: 51234})
	testifyassert.Equal(t, "10.0.0.9", addr)
	testifyassert.Equal(t, uint32(51234), port)

	// forwarded-tcpip has no spelling for "unknown", so something has to be sent.
	addr, port = originOf(&net.UnixAddr{Name: "/tmp/s", Net: "unix"})
	testifyassert.Equal(t, "127.0.0.1", addr)
	testifyassert.Equal(t, uint32(0), port)
}
