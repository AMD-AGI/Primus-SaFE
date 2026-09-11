/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
)

// sessionPair connects an initiating session to an accepting one over an
// in-process pipe, which is the whole transport an exec stream amounts to.
func sessionPair(t *testing.T, cfg Config) (initiator, acceptor *Session) {
	t.Helper()
	return sessionPairWith(t, cfg, cfg)
}

// sessionPairWith connects two ends configured differently, which is how the
// accepting end's own limits are reached: with matching caps the initiator refuses
// first and the other end's answer is never exercised.
func sessionPairWith(t *testing.T, initiatorCfg, acceptorCfg Config) (initiator, acceptor *Session) {
	t.Helper()
	a, b := net.Pipe()
	initiatorCfg.Initiator = true
	acceptorCfg.Initiator = false
	initiator = NewSession(a, initiatorCfg)
	acceptor = NewSession(b, acceptorCfg)
	t.Cleanup(func() {
		_ = initiator.Close()
		_ = acceptor.Close()
	})
	return initiator, acceptor
}

// accept takes the next stream, failing the test rather than hanging forever.
func accept(t *testing.T, s *Session) *Stream {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := s.Accept(ctx)
	testifyassert.NoError(t, err)
	if st == nil {
		t.Fatal("no stream was accepted")
	}
	return st
}

func TestStreamCarriesBytesBothWays(t *testing.T) {
	client, server := sessionPair(t, Config{})

	out, err := client.Open("10.0.0.9", 51234)
	testifyassert.NoError(t, err)

	in := accept(t, server)
	testifyassert.Equal(t, "10.0.0.9", in.OriginAddr())
	testifyassert.Equal(t, uint32(51234), in.OriginPort())
	testifyassert.Equal(t, out.ID(), in.ID())

	_, err = out.Write([]byte("request"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("request"))
	_, err = io.ReadFull(in, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "request", string(buf))

	_, err = in.Write([]byte("reply"))
	testifyassert.NoError(t, err)
	back := make([]byte, len("reply"))
	_, err = io.ReadFull(out, back)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "reply", string(back))
}

// TestHalfCloseLetsTheReplyThrough is the property socat's -t option was there to
// buy: one side saying it has finished writing must not end the other direction,
// or every request/response exchange is truncated at the reply.
func TestHalfCloseLetsTheReplyThrough(t *testing.T) {
	client, server := sessionPair(t, Config{})

	out, err := client.Open("127.0.0.1", 4242)
	testifyassert.NoError(t, err)
	in := accept(t, server)

	_, err = out.Write([]byte("GET /\r\n"))
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, out.CloseWrite())

	// The reader still drains what arrived before the fin, and only then sees it.
	got, err := io.ReadAll(in)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "GET /\r\n", string(got))

	// And the direction that was not closed still carries the reply.
	_, err = in.Write([]byte("HTTP/1.1 200 OK"))
	testifyassert.NoError(t, err)
	testifyassert.NoError(t, in.CloseWrite())

	reply, err := io.ReadAll(out)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "HTTP/1.1 200 OK", string(reply))
}

// TestWriteAfterCloseWriteIsRefused pins that a half-close is final for that
// direction, rather than silently reopening on the next write.
func TestWriteAfterCloseWriteIsRefused(t *testing.T) {
	client, server := sessionPair(t, Config{})
	out, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	defer accept(t, server).Close()

	testifyassert.NoError(t, out.CloseWrite())
	_, err = out.Write([]byte("late"))
	testifyassert.ErrorIs(t, err, ErrStreamClosed)
}

// TestStreamCapRefusesAndRecovers covers the overload contract: at the cap a new
// stream is refused straight away rather than queued, and the session goes on
// carrying the streams it already has - and takes a new one once room is freed.
func TestStreamCapRefusesAndRecovers(t *testing.T) {
	client, server := sessionPair(t, Config{MaxStreams: 2})

	first, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	second, err := client.Open("127.0.0.1", 2)
	testifyassert.NoError(t, err)
	firstIn, secondIn := accept(t, server), accept(t, server)

	_, err = client.Open("127.0.0.1", 3)
	testifyassert.ErrorIs(t, err, ErrTooManyStreams)

	// The refusal is immediate, and the streams already running are untouched.
	_, err = first.Write([]byte("still here"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("still here"))
	_, err = io.ReadFull(firstIn, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "still here", string(buf))

	testifyassert.NoError(t, second.Close())
	_ = secondIn.Close()
	waitFor(t, func() bool { return client.NumStreams() < 2 }, "the closed stream to free its slot")

	third, err := client.Open("127.0.0.1", 4)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, uint32(4), accept(t, server).OriginPort())
	_ = third.Close()
}

// TestResetEndsOneStreamAndNotTheSession is the same property from the other
// direction: an aborted connection must not take the forward down with it.
func TestResetEndsOneStreamAndNotTheSession(t *testing.T) {
	client, server := sessionPair(t, Config{})

	doomed, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	doomedIn := accept(t, server)
	survivor, err := client.Open("127.0.0.1", 2)
	testifyassert.NoError(t, err)
	survivorIn := accept(t, server)

	testifyassert.NoError(t, doomed.Close())

	_, err = io.ReadAll(doomedIn)
	testifyassert.ErrorIs(t, err, ErrStreamReset)

	_, err = survivor.Write([]byte("unaffected"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("unaffected"))
	_, err = io.ReadFull(survivorIn, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "unaffected", string(buf))
	testifyassert.NoError(t, client.Err())
}

// TestLargeTransferCrossesTheWindow moves several windows' worth of bytes, which
// only completes if credit is returned as the reader consumes.
func TestLargeTransferCrossesTheWindow(t *testing.T) {
	client, server := sessionPair(t, Config{})

	out, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	in := accept(t, server)

	payload := bytes.Repeat([]byte("0123456789abcdef"), 5*initialWindow/16)
	go func() {
		_, _ = out.Write(payload)
		_ = out.CloseWrite()
	}()

	got, err := io.ReadAll(in)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, len(payload), len(got))
	testifyassert.True(t, bytes.Equal(payload, got))
}

// TestAStalledStreamDoesNotBlockTheOthers is why the framing carries per-stream
// credit at all. One connection whose reader has wandered off must not stop every
// other connection sharing the exec stream - which is exactly the failure the mux
// replaces.
func TestAStalledStreamDoesNotBlockTheOthers(t *testing.T) {
	client, server := sessionPair(t, Config{})

	stalled, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	stalledIn := accept(t, server)
	defer stalledIn.Close()

	// Fill the stalled stream's window and leave it unread.
	filled := make(chan struct{})
	go func() {
		defer close(filled)
		_, _ = stalled.Write(bytes.Repeat([]byte("x"), 4*initialWindow))
	}()
	waitFor(t, func() bool {
		stalledIn.mu.Lock()
		defer stalledIn.mu.Unlock()
		return stalledIn.buf.Len() >= initialWindow
	}, "the stalled stream's window to fill")

	live, err := client.Open("127.0.0.1", 2)
	testifyassert.NoError(t, err)
	liveIn := accept(t, server)
	_, err = live.Write([]byte("straight through"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("straight through"))
	_, err = io.ReadFull(liveIn, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "straight through", string(buf))

	// Draining the stalled stream releases its writer.
	go func() { _, _ = io.Copy(io.Discard, stalledIn) }()
	select {
	case <-filled:
	case <-time.After(10 * time.Second):
		t.Fatal("the stalled stream's writer never finished once its reader caught up")
	}
}

// TestSessionCloseReleasesEveryStream pins that teardown is prompt: nothing waits
// on a stream whose session has gone.
func TestSessionCloseReleasesEveryStream(t *testing.T) {
	client, server := sessionPair(t, Config{})

	var streams []*Stream
	for i := 0; i < 8; i++ {
		st, err := client.Open("127.0.0.1", uint32(i+1))
		testifyassert.NoError(t, err)
		streams = append(streams, st)
		accept(t, server)
	}

	var blocked sync.WaitGroup
	for _, st := range streams {
		blocked.Add(1)
		go func(st *Stream) {
			defer blocked.Done()
			_, _ = io.ReadAll(st)
		}(st)
	}

	testifyassert.NoError(t, client.Close())
	done := make(chan struct{})
	go func() { blocked.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("readers were still waiting after the session closed")
	}
	_, err := client.Open("127.0.0.1", 99)
	testifyassert.Error(t, err)
}

// TestPeerGoingAwayEndsTheSession is the apiserver-side half of "the listen port
// is free promptly": the pod side sees the connection end and stops.
func TestPeerGoingAwayEndsTheSession(t *testing.T) {
	client, server := sessionPair(t, Config{})
	testifyassert.NoError(t, server.Close())
	select {
	case <-client.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the session outlived its peer")
	}
	testifyassert.Error(t, client.Err())
}

// TestIdleTimeoutStopsAnAbandonedSession covers the orphan case the socat relay
// had no answer for: an exec stream that is neither readable nor closed leaves the
// pod holding a listen port until the pod dies.
func TestIdleTimeoutStopsAnAbandonedSession(t *testing.T) {
	a, b := net.Pipe()
	// b is never read from and never closed, which is what an abandoned peer looks
	// like from here.
	t.Cleanup(func() { _ = b.Close() })
	s := NewSession(a, Config{IdleTimeout: 200 * time.Millisecond})
	t.Cleanup(func() { _ = s.Close() })

	select {
	case <-s.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("an abandoned session never timed out")
	}
	testifyassert.ErrorContains(t, s.Err(), "no frame from the peer")
}

// TestKeepaliveIsAnswered pins that a quiet but live peer keeps the session up.
//
// The observation window is several idle timeouts long on purpose: with keepalives
// off, or unanswered, both ends would be gone well before it ends, so the test
// fails rather than merely being slow.
func TestKeepaliveIsAnswered(t *testing.T) {
	const idle = 100 * time.Millisecond
	client, server := sessionPair(t, Config{
		KeepaliveInterval: 10 * time.Millisecond,
		IdleTimeout:       idle,
	})
	select {
	case <-client.Done():
		t.Fatalf("session ended while its peer was answering: %v", client.Err())
	case <-server.Done():
		t.Fatalf("session ended while its peer was answering: %v", server.Err())
	case <-time.After(10 * idle):
	}
}

// TestTheAcceptingEndRefusesPastItsOwnCap covers the refusal the pod side turns
// into a reset TCP connection. It is reachable only when this end is the stricter
// one, which is why the caps here differ.
func TestTheAcceptingEndRefusesPastItsOwnCap(t *testing.T) {
	client, server := sessionPairWith(t,
		Config{MaxStreams: 8},
		Config{MaxStreams: 2})

	first, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	second, err := client.Open("127.0.0.1", 2)
	testifyassert.NoError(t, err)
	accept(t, server)
	accept(t, server)

	// The initiator has room, so the open goes out; the answer is the other end's.
	refused, err := client.Open("127.0.0.1", 3)
	testifyassert.NoError(t, err)
	_, err = io.ReadAll(refused)
	testifyassert.ErrorIs(t, err, ErrStreamReset)
	testifyassert.ErrorContains(t, err, "stream limit reached")

	_, _, dropped := server.Stats()
	testifyassert.Equal(t, uint64(1), dropped)

	// The refusal is one connection's, not the session's.
	testifyassert.NoError(t, server.Err())
	_, err = first.Write([]byte("unaffected"))
	testifyassert.NoError(t, err)
	_ = second.Close()
}

// TestARefusalOutranksAFullControlQueue covers the overloaded writer path: health
// traffic may be dropped, but the reset that ends a refused stream must still get
// through as soon as the writer can make progress.
func TestARefusalOutranksAFullControlQueue(t *testing.T) {
	client, server := sessionPairWith(t,
		Config{MaxStreams: 2},
		Config{MaxStreams: 1})

	first, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	firstIn := accept(t, server)

	server.writeMu.Lock()
	writeLocked := true
	defer func() {
		if writeLocked {
			server.writeMu.Unlock()
		}
	}()

	// Let the writer take one regular frame and block on writeMu, then fill every
	// remaining regular queue slot. The refusal below must not join or be dropped
	// from this queue.
	server.enqueueCtrl(header{typ: framePong}, nil)
	waitFor(t, func() bool { return len(server.ctrl) == 0 }, "the control writer to block")
	for i := 0; i < cap(server.ctrl); i++ {
		server.ctrl <- ctrlFrame{h: header{typ: framePong}}
	}
	testifyassert.Equal(t, cap(server.ctrl), len(server.ctrl))

	refused, err := client.Open("127.0.0.1", 2)
	testifyassert.NoError(t, err)
	waitFor(t, func() bool { return droppedOf(server) == 1 }, "the accepting end to refuse the stream")
	waitFor(t, func() bool { return len(server.teardown) == 1 }, "the refusal reset to be queued")

	server.writeMu.Unlock()
	writeLocked = false

	readDone := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(refused)
		readDone <- err
	}()
	select {
	case err := <-readDone:
		testifyassert.ErrorIs(t, err, ErrStreamReset)
		testifyassert.ErrorContains(t, err, "stream limit reached")
	case <-time.After(10 * time.Second):
		t.Fatal("the refused stream stayed open after the writer resumed")
	}

	_, err = first.Write([]byte("unaffected"))
	testifyassert.NoError(t, err)
	buf := make([]byte, len("unaffected"))
	_, err = io.ReadFull(firstIn, buf)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "unaffected", string(buf))
}

// TestAbandonedConnectionsDoNotFillTheAcceptQueue covers a burst the peer gives up
// on before anything takes it. Those hold a place in the queue without carrying
// anything, so without clearing them out a session with nothing in flight would
// start refusing live connections.
func TestAbandonedConnectionsDoNotFillTheAcceptQueue(t *testing.T) {
	const cap = 4
	client, server := sessionPairWith(t,
		Config{MaxStreams: 64},
		Config{MaxStreams: cap})

	for i := 0; i < cap*3; i++ {
		st, err := client.Open("127.0.0.1", uint32(i+1))
		testifyassert.NoError(t, err)
		// Opened and given up on before anyone accepts it.
		waitFor(t, func() bool { return server.NumStreams() > 0 }, "the open to arrive")
		testifyassert.NoError(t, st.Close())
		waitFor(t, func() bool { return server.NumStreams() == 0 }, "the reset to arrive")
	}
	testifyassert.Equal(t, uint64(0), droppedOf(server),
		"connections the peer abandoned are not refusals")

	// A live connection still gets through.
	live, err := client.Open("127.0.0.1", 9999)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, uint32(9999), accept(t, server).OriginPort())
	_ = live.Close()
}

// droppedOf reads the refusal counter.
func droppedOf(s *Session) uint64 {
	_, _, dropped := s.Stats()
	return dropped
}

// TestOpenIsRefusedOnTheAcceptingEnd keeps stream ids under one owner, so two
// ends cannot allocate the same id for different connections.
func TestOpenIsRefusedOnTheAcceptingEnd(t *testing.T) {
	_, server := sessionPair(t, Config{})
	_, err := server.Open("127.0.0.1", 1)
	testifyassert.Error(t, err)
}

// TestOverrunningTheWindowEndsTheSession covers a peer that ignores the credit it
// was given: the session stops rather than buffering without limit.
func TestOverrunningTheWindowEndsTheSession(t *testing.T) {
	a, b := net.Pipe()
	victim := NewSession(a, Config{})
	t.Cleanup(func() { _ = victim.Close() })
	t.Cleanup(func() { _ = b.Close() })

	go func() {
		buf := frameBuffer()
		payload, _ := encodeOpen("127.0.0.1", 1)
		if err := writeFrame(b, buf, header{typ: frameOpen, stream: 1}, payload); err != nil {
			return
		}
		chunk := make([]byte, maxFramePayload)
		for i := 0; i < initialWindow/maxFramePayload+2; i++ {
			if err := writeFrame(b, buf, header{typ: frameData, stream: 1}, chunk); err != nil {
				return
			}
		}
	}()

	select {
	case <-victim.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("a peer that ignored its window was allowed to keep sending")
	}
	testifyassert.ErrorContains(t, victim.Err(), "overran its")
}

// TestUnknownFrameTypeEndsTheSession pins that the framing is closed: a peer
// speaking something else is stopped rather than half-understood.
func TestUnknownFrameTypeEndsTheSession(t *testing.T) {
	a, b := net.Pipe()
	victim := NewSession(a, Config{})
	t.Cleanup(func() { _ = victim.Close() })
	t.Cleanup(func() { _ = b.Close() })

	go func() { _ = writeFrame(b, frameBuffer(), header{typ: frameType(200), stream: 1}, nil) }()
	select {
	case <-victim.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("an unknown frame type was accepted")
	}
	testifyassert.ErrorContains(t, victim.Err(), "unknown frame type")
}

// TestAcceptSkipsAConnectionAlreadyGone keeps a connection the peer gave up on
// before anyone took it from being handed over: the caller would open a channel to
// its own client only to find the other end had already ended.
func TestAcceptSkipsAConnectionAlreadyGone(t *testing.T) {
	client, server := sessionPair(t, Config{})

	abandoned, err := client.Open("127.0.0.1", 7)
	testifyassert.NoError(t, err)
	waitFor(t, func() bool { return server.NumStreams() == 1 }, "the stream to reach the accepting end")
	testifyassert.NoError(t, abandoned.Close())
	waitFor(t, func() bool { return server.NumStreams() == 0 }, "the reset to reach the accepting end")

	wanted, err := client.Open("127.0.0.1", 8)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, uint32(8), accept(t, server).OriginPort())
	_ = wanted.Close()
}

// TestAcceptReportsWhyTheSessionStopped pins that a session ending is answered with
// its reason rather than with a connection that has no transport left to carry it.
func TestAcceptReportsWhyTheSessionStopped(t *testing.T) {
	client, server := sessionPair(t, Config{})
	_, err := client.Open("127.0.0.1", 7)
	testifyassert.NoError(t, err)
	waitFor(t, func() bool { return server.NumStreams() == 1 }, "the stream to reach the accepting end")

	testifyassert.NoError(t, client.Close())
	<-server.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := server.Accept(ctx)
	testifyassert.Error(t, err)
	testifyassert.Nil(t, st)
}

// TestStatsCountRefusals gives the operator the two numbers the plan asks the pod
// side to log.
func TestStatsCountRefusals(t *testing.T) {
	client, _ := sessionPair(t, Config{MaxStreams: 1})
	_, err := client.Open("127.0.0.1", 1)
	testifyassert.NoError(t, err)
	_, err = client.Open("127.0.0.1", 2)
	testifyassert.ErrorIs(t, err, ErrTooManyStreams)
	client.CountDropped()

	opened, open, dropped := client.Stats()
	testifyassert.Equal(t, uint64(1), opened)
	testifyassert.Equal(t, 1, open)
	testifyassert.Equal(t, uint64(1), dropped)
}

func TestEncodeOpenRejectsAnOverlongAddress(t *testing.T) {
	_, err := encodeOpen(string(bytes.Repeat([]byte("a"), maxOriginAddr+1)), 1)
	testifyassert.Error(t, err)
}

func TestDecodeOpenNamesAnUnknownOrigin(t *testing.T) {
	payload, err := encodeOpen("", 0)
	testifyassert.NoError(t, err)
	addr, port, err := decodeOpen(payload)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "127.0.0.1", addr)
	testifyassert.Equal(t, uint32(0), port)

	_, _, err = decodeOpen([]byte{1})
	testifyassert.Error(t, err)
}

func TestJoinClosesBothHalves(t *testing.T) {
	r, w := io.Pipe()
	conn := Join(r, io.Discard, r, w)
	testifyassert.NoError(t, conn.Close())
	_, err := w.Write([]byte("x"))
	testifyassert.True(t, errors.Is(err, io.ErrClosedPipe))
}

// waitFor polls until cond holds, so a test states what it is waiting for rather
// than sleeping for a guess.
func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestDecodeOpenRefusesAnOverlongOrigin keeps a peer from spending the whole frame
// on an origin address. The peer is a program inside a user's container and the
// address travels on to the user's SSH client, so it is bounded where it is parsed.
func TestDecodeOpenRefusesAnOverlongOrigin(t *testing.T) {
	payload := append([]byte{0, 1}, bytes.Repeat([]byte("a"), maxOriginAddr+1)...)
	_, _, err := decodeOpen(payload)
	testifyassert.ErrorContains(t, err, "over the")
}

// TestDecodeOpenRejectsAnOriginThatIsNotAnAddress covers the same value reaching a
// developer's terminal through a forwarded-tcpip channel open: whatever the peer
// spells it as, what leaves here is an address.
func TestDecodeOpenRejectsAnOriginThatIsNotAnAddress(t *testing.T) {
	payload := append([]byte{0, 1}, []byte("\x1b]0;pwned\x07")...)
	addr, port, err := decodeOpen(payload)
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "127.0.0.1", addr)
	testifyassert.Equal(t, uint32(1), port)

	addr, _, err = decodeOpen(append([]byte{0, 1}, []byte("2001:db8::1")...))
	testifyassert.NoError(t, err)
	testifyassert.Equal(t, "2001:db8::1", addr)
}

// TestResetReasonIsBoundedAndPrintable keeps a peer's diagnostic from becoming an
// arbitrarily long line of control characters in the apiserver's log.
func TestResetReasonIsBoundedAndPrintable(t *testing.T) {
	reason := safeReason(append([]byte("clean\x00\x1b[2Jtext"), bytes.Repeat([]byte("x"), maxFramePayload)...))
	testifyassert.LessOrEqual(t, len(reason), maxResetReason)
	testifyassert.True(t, strings.HasPrefix(reason, "clean[2Jtext"),
		"the text survives; only what could drive a terminal is dropped")
	testifyassert.NotContains(t, reason, "\x1b")
	testifyassert.NotContains(t, reason, "\x00")
}

// TestAHostileOriginDoesNotReachTheAcceptingEnd is the same property end to end: a
// peer that opens a stream with a made-up origin has it replaced before anyone here
// can pass it on.
func TestAHostileOriginDoesNotReachTheAcceptingEnd(t *testing.T) {
	a, b := net.Pipe()
	server := NewSession(a, Config{})
	t.Cleanup(func() { _ = server.Close(); _ = b.Close() })

	go func() {
		payload := append([]byte{0xC0, 0xDE}, []byte("\x1b[31mnot-an-address")...)
		_ = writeFrame(b, frameBuffer(), header{typ: frameOpen, stream: 1}, payload)
	}()

	st := accept(t, server)
	testifyassert.Equal(t, "127.0.0.1", st.OriginAddr())
	testifyassert.Equal(t, uint32(0xC0DE), st.OriginPort())
}
