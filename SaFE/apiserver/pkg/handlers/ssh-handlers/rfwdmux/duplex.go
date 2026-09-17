/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

import (
	"io"
	"sync"
)

// duplex presents a separate reader and writer as the one connection a session
// runs over. Both ends of a reverse forward have exactly that shape: an exec
// stream is a stdin and a stdout, not a socket.
type duplex struct {
	r         io.Reader
	w         io.Writer
	closers   []io.Closer
	closeOnce sync.Once
	closeErr  error
}

// Join builds a connection from a reader, a writer, and whatever has to be closed
// to make a blocked read on that reader return.
func Join(r io.Reader, w io.Writer, closers ...io.Closer) io.ReadWriteCloser {
	return &duplex{r: r, w: w, closers: closers}
}

func (d *duplex) Read(p []byte) (int, error)  { return d.r.Read(p) }
func (d *duplex) Write(p []byte) (int, error) { return d.w.Write(p) }

func (d *duplex) Close() error {
	d.closeOnce.Do(func() {
		for _, c := range d.closers {
			if err := c.Close(); err != nil && d.closeErr == nil {
				d.closeErr = err
			}
		}
	})
	return d.closeErr
}
