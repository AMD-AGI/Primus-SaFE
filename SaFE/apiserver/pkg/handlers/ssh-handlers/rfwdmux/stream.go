/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

// Stream is one connection carried over a session. It behaves like a TCP
// connection: bytes in both directions, either end able to say it has finished
// writing while it goes on reading, and an abort that ends both directions.
type Stream struct {
	s          *Session
	id         uint32
	originAddr string
	originPort uint32

	// rmu and wmu serialise the application's own calls, so the window bookkeeping
	// below only ever has one reader and one writer to answer to.
	rmu sync.Mutex
	wmu sync.Mutex

	mu         sync.Mutex
	buf        bytes.Buffer
	unacked    int
	sendWindow int
	peerFin    bool
	finSent    bool
	terminated bool
	failure    error
	readNotify chan struct{}
	winNotify  chan struct{}

	closed    chan struct{}
	closeOnce sync.Once
}

func newStream(s *Session, id uint32, originAddr string, originPort uint32) *Stream {
	return &Stream{
		s:          s,
		id:         id,
		originAddr: originAddr,
		originPort: originPort,
		sendWindow: initialWindow,
		readNotify: make(chan struct{}, 1),
		winNotify:  make(chan struct{}, 1),
		closed:     make(chan struct{}),
	}
}

// ID is the stream's identifier on the session.
func (st *Stream) ID() uint32 { return st.id }

// OriginAddr is the address of the peer whose connection opened this stream.
func (st *Stream) OriginAddr() string { return st.originAddr }

// OriginPort is the port of the peer whose connection opened this stream.
func (st *Stream) OriginPort() uint32 { return st.originPort }

// Read returns bytes the peer sent. It reports io.EOF once the peer has finished
// writing and everything it sent has been handed over.
func (st *Stream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	st.rmu.Lock()
	defer st.rmu.Unlock()
	for {
		st.mu.Lock()
		if st.buf.Len() > 0 {
			n, _ := st.buf.Read(p)
			st.unacked += n
			ack := 0
			if st.unacked >= windowUpdateThreshold {
				ack, st.unacked = st.unacked, 0
			}
			st.mu.Unlock()
			if ack > 0 {
				// Best effort: a window update that cannot be sent means the
				// session is going, which the next read reports on its own.
				st.sendWindowUpdate(ack)
			}
			return n, nil
		}
		// Buffered bytes outrank both of these, so a peer that sent data and then
		// stopped is fully drained before its ending is reported.
		if st.failure != nil {
			err := st.failure
			st.mu.Unlock()
			return 0, err
		}
		if st.peerFin {
			st.mu.Unlock()
			return 0, io.EOF
		}
		notifyCh := st.readNotify
		st.mu.Unlock()

		select {
		case <-notifyCh:
		case <-st.closed:
		}
	}
}

// Write sends bytes to the peer, waiting when the peer's window is exhausted.
func (st *Stream) Write(p []byte) (int, error) {
	st.wmu.Lock()
	defer st.wmu.Unlock()
	total := 0
	for len(p) > 0 {
		st.mu.Lock()
		if st.failure != nil {
			err := st.failure
			st.mu.Unlock()
			return total, err
		}
		if st.finSent {
			st.mu.Unlock()
			return total, ErrStreamClosed
		}
		if st.sendWindow > 0 {
			n := st.sendWindow
			if n > len(p) {
				n = len(p)
			}
			if n > maxFramePayload {
				n = maxFramePayload
			}
			st.sendWindow -= n
			st.mu.Unlock()
			if err := st.s.writeFrame(header{typ: frameData, stream: st.id}, p[:n]); err != nil {
				return total, err
			}
			total += n
			p = p[n:]
			continue
		}
		notifyCh := st.winNotify
		st.mu.Unlock()

		select {
		case <-notifyCh:
		case <-st.closed:
			return total, st.terminalErr()
		}
	}
	return total, nil
}

// CloseWrite reports that nothing further will be sent, leaving what the peer
// still has to say on its way.
func (st *Stream) CloseWrite() error {
	st.wmu.Lock()
	defer st.wmu.Unlock()
	st.mu.Lock()
	if st.failure != nil {
		err := st.failure
		st.mu.Unlock()
		return err
	}
	if st.finSent {
		st.mu.Unlock()
		return nil
	}
	st.finSent = true
	retire := st.peerFin
	st.mu.Unlock()

	err := st.s.writeFrame(header{typ: frameFin, stream: st.id}, nil)
	// Both directions are finished, so the stream can give its slot back without
	// waiting for the application to get around to closing it.
	if retire {
		st.s.removeStream(st.id)
	}
	return err
}

// Close ends the stream in both directions. A stream that has not finished
// exchanging fins is aborted, so the peer's connection fails rather than waiting
// on bytes that are not coming.
func (st *Stream) Close() error {
	st.mu.Lock()
	if st.terminated {
		st.mu.Unlock()
		return nil
	}
	graceful := st.finSent && st.peerFin
	st.mu.Unlock()

	st.s.removeStream(st.id)
	if !graceful {
		_ = st.s.writeFrame(header{typ: frameReset, stream: st.id}, nil)
	}
	st.terminate(ErrStreamClosed)
	return nil
}

// deliver buffers a data frame. A peer that sends past the window it was given, or
// after saying it had finished, has broken the protocol and ends the session.
func (st *Stream) deliver(payload []byte) error {
	st.mu.Lock()
	if st.terminated {
		st.mu.Unlock()
		return nil
	}
	if st.peerFin {
		st.mu.Unlock()
		return fmt.Errorf("rfwdmux: stream %d carried data after its fin", st.id)
	}
	if st.buf.Len()+st.unacked+len(payload) > initialWindow {
		st.mu.Unlock()
		return fmt.Errorf("rfwdmux: stream %d overran its %d byte window", st.id, initialWindow)
	}
	st.buf.Write(payload)
	st.mu.Unlock()
	notify(st.readNotify)
	return nil
}

// peerFinished records the peer's fin.
func (st *Stream) peerFinished() {
	st.mu.Lock()
	if st.peerFin {
		st.mu.Unlock()
		return
	}
	st.peerFin = true
	retire := st.finSent
	st.mu.Unlock()
	notify(st.readNotify)
	if retire {
		st.s.removeStream(st.id)
	}
}

// grantWindow returns credit the peer's application has consumed.
func (st *Stream) grantWindow(n int) {
	if n <= 0 {
		return
	}
	st.mu.Lock()
	st.sendWindow += n
	if st.sendWindow > initialWindow {
		// An honest peer never returns more credit than it was spent. Clamping
		// keeps a peer from inflating this into writes that overrun its own window
		// - which it would then end the session over - and keeps the counter away
		// from an overflow that would stall this stream's writer for good.
		st.sendWindow = initialWindow
	}
	st.mu.Unlock()
	notify(st.winNotify)
}

// sendWindowUpdate tells the peer it may send n more bytes.
func (st *Stream) sendWindowUpdate(n int) {
	var payload [4]byte
	binary.BigEndian.PutUint32(payload[:], uint32(n))
	_ = st.s.writeFrame(header{typ: frameWindow, stream: st.id}, payload[:])
}

// terminate ends the stream locally, releasing whoever is waiting on it.
func (st *Stream) terminate(err error) {
	st.closeOnce.Do(func() {
		st.mu.Lock()
		st.terminated = true
		if st.failure == nil {
			st.failure = err
		}
		st.mu.Unlock()
		close(st.closed)
	})
}

// alive reports whether the stream can still carry anything.
func (st *Stream) alive() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return !st.terminated
}

// terminalErr reports why the stream stopped.
func (st *Stream) terminalErr() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.failure != nil {
		return st.failure
	}
	return ErrStreamClosed
}

// notify wakes a waiter without ever blocking the caller.
func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
