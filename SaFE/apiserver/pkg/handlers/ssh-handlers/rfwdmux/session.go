/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxStreams bounds how many connections one forward carries at a time.
// It is a ceiling on the pod's memory and on the framing bookkeeping, not a
// throughput target: past it the pod side refuses the TCP connection outright so
// that the process making it fails now instead of hanging.
const DefaultMaxStreams = 256

// initialWindow is how many bytes a peer may have in flight on one stream before
// the receiving application has consumed any of them. It is the per-stream memory
// cost of a stalled reader, and with DefaultMaxStreams bounds a session's buffers
// at a few tens of megabytes.
const initialWindow = 128 * 1024

// windowUpdateThreshold is the share of the window that must be consumed before
// credit is returned. Returning it on every read would spend more of the shared
// connection on bookkeeping than on payload.
const windowUpdateThreshold = initialWindow / 2

// ctrlQueueDepth bounds each class of frames the read loop may hand to the writer.
// The read loop must never block on a write - it is what drains the connection, so
// a read loop waiting on a full transport is a session that can never recover.
const ctrlQueueDepth = 128

var (
	// ErrSessionClosed reports a session that has ended.
	ErrSessionClosed = errors.New("rfwdmux: session is closed")
	// ErrTooManyStreams reports that the stream cap is reached. The pod side turns
	// it into a refused TCP connection.
	ErrTooManyStreams = errors.New("rfwdmux: stream limit reached")
	// ErrStreamReset reports a stream the peer aborted.
	ErrStreamReset = errors.New("rfwdmux: stream reset by peer")
	// ErrStreamClosed reports a stream this side has closed.
	ErrStreamClosed = errors.New("rfwdmux: stream is closed")
)

// Config settles the behaviour of one session.
type Config struct {
	// MaxStreams caps concurrent streams; zero means DefaultMaxStreams.
	MaxStreams int
	// Initiator marks the end that opens streams. The other end refuses to, and
	// treats an open it did not ask for as a protocol error, so stream ids have a
	// single owner and cannot collide.
	Initiator bool
	// KeepaliveInterval sends a ping when the connection has been quiet for this
	// long. Zero sends none.
	KeepaliveInterval time.Duration
	// IdleTimeout ends the session when nothing has arrived for this long. It is
	// what stops a pod-side mux whose apiserver has gone from holding the listen
	// port for the life of the pod. Zero waits forever.
	IdleTimeout time.Duration
	// Logf receives diagnostics. Zero discards them.
	Logf func(format string, args ...any)
}

func (c Config) maxStreams() int {
	if c.MaxStreams > 0 {
		return c.MaxStreams
	}
	return DefaultMaxStreams
}

func (c Config) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

// ctrlFrame is a frame the read loop asked the writer to send on its behalf.
type ctrlFrame struct {
	h       header
	payload []byte
}

// Session multiplexes streams over one duplex connection.
type Session struct {
	conn io.ReadWriteCloser
	cfg  Config

	// writeMu serialises frames onto the connection. A frame is written whole
	// under it, so two streams cannot interleave a header and a payload.
	writeMu  sync.Mutex
	writeBuf []byte

	ctrl     chan ctrlFrame
	teardown chan ctrlFrame

	mu      sync.Mutex
	streams map[uint32]*Stream
	nextID  uint32

	accept chan *Stream

	shutdownOnce sync.Once
	done         chan struct{}
	errMu        sync.Mutex
	err          error

	lastActivity atomic.Int64
	dropped      atomic.Uint64
	opened       atomic.Uint64
}

// NewSession starts multiplexing over conn. The session owns conn from here on and
// closes it when it ends.
func NewSession(conn io.ReadWriteCloser, cfg Config) *Session {
	s := &Session{
		conn:     conn,
		cfg:      cfg,
		writeBuf: frameBuffer(),
		ctrl:     make(chan ctrlFrame, ctrlQueueDepth),
		teardown: make(chan ctrlFrame, ctrlQueueDepth),
		streams:  map[uint32]*Stream{},
		nextID:   1,
		accept:   make(chan *Stream, cfg.maxStreams()),
		done:     make(chan struct{}),
	}
	s.lastActivity.Store(time.Now().UnixNano())
	go s.readLoop()
	go s.ctrlLoop()
	if cfg.KeepaliveInterval > 0 || cfg.IdleTimeout > 0 {
		go s.healthLoop()
	}
	return s
}

// Done closes when the session has ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// Err reports why the session ended, or nil while it is running.
func (s *Session) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

// MaxStreams is the cap this session enforces.
func (s *Session) MaxStreams() int { return s.cfg.maxStreams() }

// NumStreams reports how many streams are open right now.
func (s *Session) NumStreams() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.streams)
}

// Stats reports the counters worth logging: streams opened, streams open now, and
// connections refused because the cap was reached.
func (s *Session) Stats() (opened uint64, open int, dropped uint64) {
	return s.opened.Load(), s.NumStreams(), s.dropped.Load()
}

// CountDropped records a connection the caller refused before it became a stream.
func (s *Session) CountDropped() { s.dropped.Add(1) }

// Close ends the session and every stream on it.
func (s *Session) Close() error {
	s.shutdown(ErrSessionClosed)
	return nil
}

// shutdown ends the session once, with the first reason given.
func (s *Session) shutdown(err error) {
	s.shutdownOnce.Do(func() {
		if err == nil {
			err = ErrSessionClosed
		}
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
		close(s.done)

		s.mu.Lock()
		streams := make([]*Stream, 0, len(s.streams))
		for id, st := range s.streams {
			delete(s.streams, id)
			streams = append(streams, st)
		}
		s.mu.Unlock()
		for _, st := range streams {
			st.terminate(err)
		}
		// Closing the connection is what unblocks the read loop; without it a
		// session whose peer has stopped talking but not hung up never returns.
		_ = s.conn.Close()
	})
}

// Open starts a stream carrying a connection from origin. Only the initiating end
// may call it.
func (s *Session) Open(originAddr string, originPort uint32) (*Stream, error) {
	if !s.cfg.Initiator {
		return nil, errors.New("rfwdmux: this end of the session does not open streams")
	}
	payload, err := encodeOpen(originAddr, originPort)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	select {
	case <-s.done:
		s.mu.Unlock()
		return nil, s.Err()
	default:
	}
	if len(s.streams) >= s.cfg.maxStreams() {
		s.mu.Unlock()
		return nil, ErrTooManyStreams
	}
	id := s.nextID
	s.nextID++
	if s.nextID == 0 {
		// Zero is never a stream id, so a header full of zeroes cannot be mistaken
		// for a frame about a live stream.
		s.nextID = 1
	}
	st := newStream(s, id, originAddr, originPort)
	s.streams[id] = st
	s.mu.Unlock()

	if err = s.writeFrame(header{typ: frameOpen, stream: id}, payload); err != nil {
		s.removeStream(id)
		st.terminate(err)
		return nil, err
	}
	s.opened.Add(1)
	return st, nil
}

// Accept returns the next stream the peer opened.
func (s *Session) Accept(ctx context.Context) (*Stream, error) {
	for {
		select {
		case st := <-s.accept:
			// A connection the peer gave up on before anyone took it is not worth
			// handing over: the caller would open a channel to its client only to
			// find the other end already gone.
			if st.alive() {
				return st, nil
			}
		case <-s.done:
			// Nothing queued survives the session: ending it terminates every
			// stream, so a connection still waiting here has no transport left to
			// carry it and reporting why the session stopped is the useful answer.
			return nil, s.Err()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// writeFrame sends one frame, waiting for the connection if it is busy.
func (s *Session) writeFrame(h header, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	select {
	case <-s.done:
		return s.Err()
	default:
	}
	if err := writeFrame(s.conn, s.writeBuf, h, payload); err != nil {
		s.shutdown(fmt.Errorf("rfwdmux: write failed: %w", err))
		return err
	}
	return nil
}

// enqueueCtrl hands a best-effort frame to the writer without waiting. Pings and
// pongs keep a healthy session observable, but dropping one under backpressure
// does not leave a stream open.
func (s *Session) enqueueCtrl(h header, payload []byte) {
	select {
	case s.ctrl <- ctrlFrame{h: h, payload: payload}:
	default:
		s.cfg.logf("rfwdmux: control queue full, dropping %s for stream %d", h.typ, h.stream)
	}
}

// enqueueTeardown gives a stream-ending frame its own queue. If that queue is
// exhausted too, closing the session is the only bounded way to guarantee the
// peer does not keep the refused stream open forever.
func (s *Session) enqueueTeardown(h header, payload []byte) {
	select {
	case s.teardown <- ctrlFrame{h: h, payload: payload}:
	case <-s.done:
	default:
		s.shutdown(errors.New("rfwdmux: teardown queue full"))
	}
}

// ctrlLoop writes the frames the read loop handed over, always draining stream
// teardown before best-effort health traffic.
func (s *Session) ctrlLoop() {
	for {
		select {
		case f := <-s.teardown:
			if err := s.writeFrame(f.h, f.payload); err != nil {
				return
			}
			continue
		default:
		}

		select {
		case f := <-s.teardown:
			if err := s.writeFrame(f.h, f.payload); err != nil {
				return
			}
		case f := <-s.ctrl:
			if err := s.writeFrame(f.h, f.payload); err != nil {
				return
			}
		case <-s.done:
			return
		}
	}
}

// healthLoop keeps the session honest about being alive.
func (s *Session) healthLoop() {
	interval := s.cfg.KeepaliveInterval
	if interval <= 0 || (s.cfg.IdleTimeout > 0 && s.cfg.IdleTimeout/4 < interval) {
		interval = s.cfg.IdleTimeout / 4
	}
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			idle := time.Since(time.Unix(0, s.lastActivity.Load()))
			if s.cfg.IdleTimeout > 0 && idle > s.cfg.IdleTimeout {
				s.shutdown(fmt.Errorf("rfwdmux: no frame from the peer for %s", idle.Truncate(time.Second)))
				return
			}
			if s.cfg.KeepaliveInterval > 0 {
				s.enqueueCtrl(header{typ: framePing}, nil)
			}
		}
	}
}

// readLoop consumes frames until the connection ends.
func (s *Session) readLoop() {
	buf := make([]byte, maxFramePayload)
	hdr := make([]byte, headerSize)
	for {
		if _, err := io.ReadFull(s.conn, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				err = io.EOF
			}
			s.shutdown(err)
			return
		}
		h := decodeHeader(hdr)
		if int(h.length) > maxFramePayload {
			s.shutdown(fmt.Errorf("rfwdmux: %s frame declares %d bytes, over the limit", h.typ, h.length))
			return
		}
		payload := buf[:h.length]
		if h.length > 0 {
			if _, err := io.ReadFull(s.conn, payload); err != nil {
				s.shutdown(err)
				return
			}
		}
		s.lastActivity.Store(time.Now().UnixNano())
		if err := s.dispatch(h, payload); err != nil {
			s.shutdown(err)
			return
		}
	}
}

// dispatch acts on one frame. A returned error is a protocol violation and ends
// the session; anything a well-behaved peer can cause is handled in place.
func (s *Session) dispatch(h header, payload []byte) error {
	switch h.typ {
	case frameOpen:
		return s.handleOpen(h, payload)
	case frameData:
		if st := s.stream(h.stream); st != nil {
			return st.deliver(payload)
		}
		// A stream this end has already finished with: the peer's frames were in
		// flight when it was closed, and it has our reset by now.
		return nil
	case frameFin:
		if st := s.stream(h.stream); st != nil {
			st.peerFinished()
		}
		return nil
	case frameReset:
		if st := s.stream(h.stream); st != nil {
			s.removeStream(h.stream)
			reason := safeReason(payload)
			if reason == "" {
				st.terminate(ErrStreamReset)
			} else {
				st.terminate(fmt.Errorf("%w: %s", ErrStreamReset, reason))
			}
		}
		return nil
	case frameWindow:
		if len(payload) != 4 {
			return fmt.Errorf("rfwdmux: window frame carries %d bytes, want 4", len(payload))
		}
		if st := s.stream(h.stream); st != nil {
			st.grantWindow(int(uint32(payload[0])<<24 | uint32(payload[1])<<16 |
				uint32(payload[2])<<8 | uint32(payload[3])))
		}
		return nil
	case framePing:
		s.enqueueCtrl(header{typ: framePong}, nil)
		return nil
	case framePong:
		return nil
	default:
		return fmt.Errorf("rfwdmux: unknown frame type %d", uint8(h.typ))
	}
}

// handleOpen registers a stream the peer opened, or refuses it.
func (s *Session) handleOpen(h header, payload []byte) error {
	if s.cfg.Initiator {
		return errors.New("rfwdmux: peer opened a stream on the initiating end")
	}
	if h.stream == 0 {
		return errors.New("rfwdmux: peer opened stream 0")
	}
	addr, port, err := decodeOpen(payload)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if _, exists := s.streams[h.stream]; exists {
		s.mu.Unlock()
		return fmt.Errorf("rfwdmux: peer reopened stream %d", h.stream)
	}
	if len(s.streams) >= s.cfg.maxStreams() {
		s.mu.Unlock()
		s.refuse(h.stream, "stream limit reached")
		return nil
	}
	st := newStream(s, h.stream, addr, port)
	s.streams[h.stream] = st
	s.mu.Unlock()

	if s.queueForAccept(st) {
		s.opened.Add(1)
		return nil
	}
	// Nothing is taking connections off this session fast enough. Refusing is the
	// honest answer: the process inside the pod sees its connection fail instead of
	// waiting on one that will never be served.
	s.removeStream(h.stream)
	s.refuse(h.stream, "accept queue full")
	return nil
}

// queueForAccept puts a stream in front of Accept, reporting whether there was room.
//
// A queue that is full is not the same as a session that is busy: a peer can open
// connections and abandon them before anyone takes them, and those hold a place
// without carrying anything. Clearing those out first is what keeps a burst of
// abandoned connections from making the session refuse live ones.
//
// Only the read loop reaches this, so it is the sole producer on the queue.
func (s *Session) queueForAccept(st *Stream) bool {
	select {
	case s.accept <- st:
		return true
	default:
	}
	for i := 0; i < cap(s.accept); i++ {
		select {
		case queued := <-s.accept:
			if queued.alive() {
				// Still worth handing over. The order connections are accepted in
				// is not a contract; losing one would be.
				s.accept <- queued
				return false
			}
		default:
			return false
		}
		select {
		case s.accept <- st:
			return true
		default:
		}
	}
	return false
}

// refuse tells the peer a stream will not be carried.
func (s *Session) refuse(id uint32, reason string) {
	s.dropped.Add(1)
	s.cfg.logf("rfwdmux: refused stream %d: %s", id, reason)
	s.enqueueTeardown(header{typ: frameReset, stream: id}, []byte(reason))
}

// stream looks up a live stream.
func (s *Session) stream(id uint32) *Stream {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[id]
}

// removeStream drops a stream from the table.
func (s *Session) removeStream(id uint32) {
	s.mu.Lock()
	delete(s.streams, id)
	s.mu.Unlock()
}
