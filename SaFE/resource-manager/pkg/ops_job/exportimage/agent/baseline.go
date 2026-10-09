/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// baselineHeader starts every baseline file, so that a file of another format (or a
// truncated one) is never read as a list of what the image held.
const baselineHeader = "primus-safe save-image baseline v1\n"

// baselineTrailer ends a complete baseline file.
const baselineTrailer = "\x00end\x00"

// Record lists the file system under root and records it in file. The platform launcher
// runs it before anything else runs in the container: the export measures deletions
// against it, instead of reading every layer of the image from the registry. Any earlier
// record is removed first, so a record that fails leaves none rather than one an earlier
// container (of the same Pod, on the same shared volume) left. It returns the number of
// paths recorded.
func Record(file, root, mountinfo string) (int, error) {
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	entries, err := Walk(root, NewFilter(ParseMountPoints(mountinfo)))
	if err != nil {
		return 0, err
	}
	return len(entries), WriteBaseline(file, entries)
}

// WriteBaseline writes the record of the paths the container started with, atomically.
func WriteBaseline(file string, entries []Entry) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	w := bufio.NewWriterSize(tmp, 1<<20)
	_, _ = w.WriteString(baselineHeader)
	for _, e := range entries {
		_, _ = w.WriteString(e.Path)
		_ = w.WriteByte(0)
	}
	_, _ = w.WriteString(baselineTrailer)
	if err := w.Flush(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// ReadBaseline reads what WriteBaseline recorded.
func ReadBaseline(r io.Reader) (map[string]bool, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte(baselineHeader)) || !bytes.HasSuffix(data, []byte(baselineTrailer)) {
		return nil, fmt.Errorf("the record of the image's files is incomplete or of an unknown format")
	}
	body := data[len(baselineHeader) : len(data)-len(baselineTrailer)]
	set := map[string]bool{}
	for len(body) > 0 {
		i := bytes.IndexByte(body, 0)
		if i < 0 {
			return nil, fmt.Errorf("the record of the image's files is malformed")
		}
		if p := string(body[:i]); p != "" {
			if p[0] != '/' {
				return nil, fmt.Errorf("the record of the image's files holds a relative path")
			}
			set[p] = true
		}
		body = body[i+1:]
	}
	return set, nil
}
