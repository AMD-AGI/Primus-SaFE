/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package rfwdmux carries many byte streams over one duplex connection.
//
// It exists because the transport underneath a reverse forward is a Kubernetes
// exec stream, which is expensive to set up and limited in number. One exec per
// forwarded TCP connection runs out of room; one exec per forward does not.
//
// The framing is deliberately small: a stream that is opened, carries bytes in
// both directions, can be half-closed from either end, and can be aborted. What
// it adds beyond that is per-stream credit, so that one stalled consumer cannot
// stop every other stream sharing the connection.
package rfwdmux

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
)

// frameType identifies what a frame carries.
type frameType uint8

const (
	// frameOpen announces a new stream. Only the initiator sends it, and the
	// payload is the address of the peer whose connection opened the stream.
	frameOpen frameType = 1
	// frameData carries stream bytes.
	frameData frameType = 2
	// frameFin says the sender will send no more data on this stream. The other
	// direction stays open, which is what a request followed by its reply needs.
	frameFin frameType = 3
	// frameReset aborts a stream in both directions, with a reason for the log.
	frameReset frameType = 4
	// frameWindow returns credit: the payload counts bytes the receiving
	// application has consumed and is therefore willing to be sent again.
	frameWindow frameType = 5
	// framePing and framePong keep an idle session provably alive. A pod-side mux
	// whose apiserver has vanished has no other way to learn it should let go of
	// the listen port.
	framePing frameType = 6
	framePong frameType = 7
)

func (t frameType) String() string {
	switch t {
	case frameOpen:
		return "OPEN"
	case frameData:
		return "DATA"
	case frameFin:
		return "FIN"
	case frameReset:
		return "RST"
	case frameWindow:
		return "WINDOW"
	case framePing:
		return "PING"
	case framePong:
		return "PONG"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", uint8(t))
	}
}

const (
	// headerSize is type(1) + stream id(4) + payload length(2).
	headerSize = 7
	// maxFramePayload bounds one frame, and with it the read buffer each end has
	// to hold. It is under the 16-bit length field the header carries.
	maxFramePayload = 32 * 1024
	// maxResetReason bounds the diagnostic text on a reset, so a peer cannot make
	// the other end log an arbitrarily long line.
	maxResetReason = 256
	// maxOriginAddr bounds the address on an open. It is generous for an IPv6
	// literal with a zone and refuses anything that is not one.
	maxOriginAddr = 128
)

// header is the fixed part of every frame.
type header struct {
	typ    frameType
	stream uint32
	length uint16
}

// encodeHeader writes h into the first headerSize bytes of buf.
func encodeHeader(buf []byte, h header) {
	buf[0] = byte(h.typ)
	binary.BigEndian.PutUint32(buf[1:5], h.stream)
	binary.BigEndian.PutUint16(buf[5:7], h.length)
}

// decodeHeader reads a header out of buf, which must be at least headerSize long.
func decodeHeader(buf []byte) header {
	return header{
		typ:    frameType(buf[0]),
		stream: binary.BigEndian.Uint32(buf[1:5]),
		length: binary.BigEndian.Uint16(buf[5:7]),
	}
}

// encodeOpen builds the payload of an open frame: the origin port followed by the
// origin address. The address is last so it needs no length of its own.
func encodeOpen(addr string, port uint32) ([]byte, error) {
	if len(addr) > maxOriginAddr {
		return nil, fmt.Errorf("origin address %q is longer than %d bytes", addr, maxOriginAddr)
	}
	buf := make([]byte, 2+len(addr))
	binary.BigEndian.PutUint16(buf[:2], uint16(port))
	copy(buf[2:], addr)
	return buf, nil
}

// decodeOpen parses an open payload.
//
// The address is bounded and then required to be an IP literal. It travels no
// further than a forwarded-tcpip channel open, which the peer's SSH client logs and
// shows; the peer is a program running in a user's container, so an address it made
// up out of escape sequences would be the apiserver relaying them to a developer's
// terminal.
func decodeOpen(payload []byte) (addr string, port uint32, err error) {
	if len(payload) < 2 {
		return "", 0, fmt.Errorf("open frame payload is %d bytes, want at least 2", len(payload))
	}
	if len(payload)-2 > maxOriginAddr {
		return "", 0, fmt.Errorf("open frame carries a %d byte origin address, over the %d limit",
			len(payload)-2, maxOriginAddr)
	}
	port = uint32(binary.BigEndian.Uint16(payload[:2]))
	addr = string(payload[2:])
	if net.ParseIP(addr) == nil {
		// An origin the peer could not name, or named as something that is not an
		// address, still has to reach the SSH client as something: forwarded-tcpip
		// has no spelling for "unknown".
		addr = "127.0.0.1"
	}
	return addr, port, nil
}

// safeReason trims a peer's diagnostic to something safe to keep and to log: the
// peer chooses these bytes, and they end up in an error string and in the log.
func safeReason(payload []byte) string {
	if len(payload) > maxResetReason {
		payload = payload[:maxResetReason]
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, string(payload))
}

// writeFrame writes one frame to w. Callers hold the session write lock, because
// a header and its payload must not be split by another stream's frame.
func writeFrame(w io.Writer, buf []byte, h header, payload []byte) error {
	if len(payload) > maxFramePayload {
		return fmt.Errorf("frame payload is %d bytes, over the %d limit", len(payload), maxFramePayload)
	}
	h.length = uint16(len(payload))
	encodeHeader(buf, h)
	// One write, not two: the transport underneath is a pipe into an exec stream,
	// and a header that reaches the peer without its payload behind it stalls the
	// peer's reader for as long as the second write takes.
	n := copy(buf[headerSize:], payload)
	_, err := w.Write(buf[:headerSize+n])
	return err
}

// frameBuffer is a scratch buffer big enough for any frame this package writes.
func frameBuffer() []byte { return make([]byte, headerSize+maxFramePayload) }
