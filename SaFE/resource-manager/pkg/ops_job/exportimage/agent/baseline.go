/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// baselineHeader starts every baseline file, so that a file of another format (or a
// truncated one) is never read as a list of what the image held.
const baselineHeader = "primus-safe save-image baseline v2\n"

// A baseline holds one record per path, in walk order:
//
//	<type><seconds>.<nanoseconds> <path>\x00
//
// where type is Entry.Type and the time is the change time the path had when it was
// listed, and ends with "E<count>\x00".
const baselineEnd = 'E'

// RecordingSuffix names the file a record is written to until it is complete. Its
// presence, without a baseline, means the record is still being made (or failed).
const RecordingSuffix = ".partial"

// Record lists the file system under root and records it in file. The platform launcher
// starts it after its own bootstrap, in the background and at low priority: the export
// measures deletions against it, instead of reading every layer of the image from the
// registry. Paths are written as they are listed, so its memory does not grow with the
// image. Any earlier record is removed first, so a record that fails leaves none rather
// than one an earlier container (of the same Pod, on the same shared volume) left. It
// returns the number of paths recorded.
func Record(file, root, mountinfo string) (int, error) {
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	tmp := file + RecordingSuffix
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	bw := NewBaselineWriter(f)
	err = Walk(root, NewFilter(ParseMountPoints(mountinfo)), bw.Add)
	if err == nil {
		err = bw.Close()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return bw.n, os.Rename(tmp, file)
}

// BaselineWriter writes a baseline as its entries come.
type BaselineWriter struct {
	w    *bufio.Writer
	n    int
	last string
}

// NewBaselineWriter starts a baseline on w.
func NewBaselineWriter(w io.Writer) *BaselineWriter {
	bw := &BaselineWriter{w: bufio.NewWriterSize(w, 1<<20)}
	_, _ = bw.w.WriteString(baselineHeader)
	return bw
}

// Add records one entry. Entries must come in walk order.
func (b *BaselineWriter) Add(e Entry) error {
	if b.n > 0 && ComparePaths(b.last, e.Path) >= 0 {
		return fmt.Errorf("%s is listed after %s", e.Path, b.last)
	}
	if !strings.HasPrefix(e.Path, "/") || strings.IndexByte(e.Path, 0) >= 0 {
		return fmt.Errorf("cannot record %q", e.Path)
	}
	b.w.WriteByte(e.Type)
	b.w.WriteString(strconv.FormatInt(e.Ctime.Sec, 10))
	b.w.WriteByte('.')
	b.w.WriteString(strconv.FormatInt(e.Ctime.Nsec, 10))
	b.w.WriteByte(' ')
	b.w.WriteString(e.Path)
	if err := b.w.WriteByte(0); err != nil {
		return err
	}
	b.n++
	b.last = e.Path
	return nil
}

// Close ends the baseline. It does not close the underlying writer.
func (b *BaselineWriter) Close() error {
	b.w.WriteByte(baselineEnd)
	b.w.WriteString(strconv.Itoa(b.n))
	b.w.WriteByte(0)
	return b.w.Flush()
}

// WriteBaseline writes a whole baseline to file; entries must be in walk order.
func WriteBaseline(file string, entries []Entry) error {
	f, err := os.OpenFile(file, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	bw := NewBaselineWriter(f)
	for _, e := range entries {
		if err := bw.Add(e); err != nil {
			f.Close()
			return err
		}
	}
	if err := bw.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

var errBaselineMalformed = errors.New("the record of the image's files is incomplete or of an unknown format")

// BaselineReader reads a baseline one entry at a time.
type BaselineReader struct {
	r    *bufio.Reader
	n    int
	last string
	done bool
}

// NewBaselineReader checks the baseline's header and returns a reader of its entries.
func NewBaselineReader(r io.Reader) (*BaselineReader, error) {
	br := bufio.NewReaderSize(r, 1<<20)
	head := make([]byte, len(baselineHeader))
	if _, err := io.ReadFull(br, head); err != nil || string(head) != baselineHeader {
		return nil, errBaselineMalformed
	}
	return &BaselineReader{r: br}, nil
}

// Next returns the next entry, or false at the end. A baseline that does not end the way
// a complete one does is an error, never a shorter list.
func (b *BaselineReader) Next() (Entry, bool, error) {
	if b.done {
		return Entry{}, false, nil
	}
	rec, err := b.r.ReadString(0)
	if err != nil {
		return Entry{}, false, errBaselineMalformed
	}
	rec = rec[:len(rec)-1]
	if rec == "" {
		return Entry{}, false, errBaselineMalformed
	}
	if rec[0] == baselineEnd {
		n, err := strconv.Atoi(rec[1:])
		if err != nil || n != b.n {
			return Entry{}, false, errBaselineMalformed
		}
		if _, err := b.r.ReadByte(); err != io.EOF {
			return Entry{}, false, errBaselineMalformed
		}
		b.done = true
		return Entry{}, false, nil
	}
	meta, p, ok := strings.Cut(rec[1:], " ")
	if !ok || !strings.HasPrefix(p, "/") {
		return Entry{}, false, errBaselineMalformed
	}
	secs, nsecs, ok := strings.Cut(meta, ".")
	if !ok {
		return Entry{}, false, errBaselineMalformed
	}
	sec, err1 := strconv.ParseInt(secs, 10, 64)
	nsec, err2 := strconv.ParseInt(nsecs, 10, 64)
	if err1 != nil || err2 != nil {
		return Entry{}, false, errBaselineMalformed
	}
	if b.n > 0 && ComparePaths(b.last, p) >= 0 {
		return Entry{}, false, fmt.Errorf("the record of the image's files is out of order at %q", p)
	}
	b.n++
	b.last = p
	return Entry{Path: p, Type: rec[0], Ctime: Timestamp{Sec: sec, Nsec: nsec}}, true, nil
}
