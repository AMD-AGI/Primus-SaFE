/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
)

// LayerStats describes a written layer.
type LayerStats struct {
	Entries   int
	Whiteouts int
	// Vanished counts changed paths that were gone by the time they were archived.
	Vanished int
	// Resized counts files whose size changed while they were read; each is archived at
	// the size it had when its header was written.
	Resized int
	Bytes   int64
	// DroppedPackages are the dpkg records left out because the package's files are not
	// in the image.
	DroppedPackages []string
}

// maxDpkgStatus bounds the package database read into memory to be reconciled.
const maxDpkgStatus = 256 << 20

// WriteLayer writes an uncompressed layer to w: the changed paths, read from the file
// system under root, then one OCI whiteout per deleted path. A changed directory carries
// its own metadata only. Ownership is numeric and only modification times are kept; a
// second name of an already archived inode becomes a hard link. When inImage is set, the
// dpkg status file is reconciled with it (see ReconcileDpkgStatus). It does not close w.
func WriteLayer(w io.Writer, root string, changed []string, deleted []string, inImage func(string) bool) (LayerStats, error) {
	var st LayerStats
	cw := &countingWriter{w: w}
	tw := tar.NewWriter(cw)
	type inode struct{ dev, ino uint64 }
	links := map[inode]string{}
	for _, abs := range changed {
		full := filepath.Join(root, filepath.FromSlash(abs))
		info, err := os.Lstat(full)
		if errors.Is(err, fs.ErrNotExist) {
			st.Vanished++
			continue
		}
		if err != nil {
			return st, err
		}
		hdr, err := headerFor(abs, full, info)
		if errors.Is(err, fs.ErrNotExist) {
			st.Vanished++
			continue
		}
		if err != nil {
			return st, err
		}
		if hdr.Typeflag == tar.TypeReg {
			if sys, ok := info.Sys().(*syscall.Stat_t); ok && sys.Nlink > 1 {
				key := inode{uint64(sys.Dev), uint64(sys.Ino)}
				if first, seen := links[key]; seen {
					hdr.Typeflag, hdr.Linkname, hdr.Size = tar.TypeLink, first, 0
				} else {
					links[key] = hdr.Name
				}
			}
		}
		if hdr.Typeflag != tar.TypeReg {
			if err := tw.WriteHeader(hdr); err != nil {
				return st, fmt.Errorf("writing %s: %w", abs, err)
			}
			st.Entries++
			continue
		}
		vanished, err := writeFile(tw, &st, abs, full, hdr, inImage)
		if err != nil {
			return st, err
		}
		if vanished {
			st.Vanished++
			continue
		}
		st.Entries++
	}
	for _, p := range deleted {
		dir, base := path.Split(strings.TrimPrefix(p, "/"))
		if base == "" {
			return st, fmt.Errorf("cannot write a whiteout for %q", p)
		}
		hdr := &tar.Header{
			Name:     dir + whiteoutPrefix + base,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Format:   tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return st, fmt.Errorf("writing whiteout for %s: %w", p, err)
		}
		st.Whiteouts++
	}
	if err := tw.Close(); err != nil {
		return st, err
	}
	st.Bytes = cw.n
	return st, nil
}

// headerFor builds the header of one path. It keeps what a layer needs and drops what
// only describes the machine the container ran on: user and group names (ownership stays
// numeric), and access and change times.
func headerFor(abs, full string, info fs.FileInfo) (*tar.Header, error) {
	var link string
	if info.Mode()&fs.ModeSymlink != 0 {
		var err error
		if link, err = os.Readlink(full); err != nil {
			return nil, err
		}
	}
	h, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return nil, fmt.Errorf("archiving %s: %w", abs, err)
	}
	out := &tar.Header{
		Typeflag: h.Typeflag,
		Name:     strings.TrimPrefix(abs, "/"),
		Linkname: h.Linkname,
		Size:     h.Size,
		Mode:     h.Mode,
		Uid:      h.Uid,
		Gid:      h.Gid,
		ModTime:  h.ModTime,
		Devmajor: h.Devmajor,
		Devminor: h.Devminor,
		Format:   tar.FormatPAX,
	}
	if out.Typeflag == tar.TypeDir {
		out.Name += "/"
	}
	return out, nil
}

// writeFile archives a regular file's contents. A file that shrank while it was read is
// padded with zeros and one that grew is cut, so the archive always matches its header.
// It reports whether the file was gone before it could be opened.
func writeFile(tw *tar.Writer, st *LayerStats, abs, full string, hdr *tar.Header, inImage func(string) bool) (bool, error) {
	// O_NOFOLLOW: a path that was a file when it was listed and is a symbolic link now
	// must not be followed to whatever it names.
	f, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", abs, err)
	}
	defer f.Close()

	if abs == DpkgStatusPath && inImage != nil {
		data, err := io.ReadAll(io.LimitReader(f, maxDpkgStatus))
		if err != nil {
			return false, fmt.Errorf("reading %s: %w", abs, err)
		}
		data, st.DroppedPackages = ReconcileDpkgStatus(data, inImage)
		hdr.Size = int64(len(data))
		if err := tw.WriteHeader(hdr); err != nil {
			return false, fmt.Errorf("writing %s: %w", abs, err)
		}
		if _, err := tw.Write(data); err != nil {
			return false, fmt.Errorf("writing %s: %w", abs, err)
		}
		return false, nil
	}

	if err := tw.WriteHeader(hdr); err != nil {
		return false, fmt.Errorf("writing %s: %w", abs, err)
	}
	n, err := io.Copy(tw, io.LimitReader(f, hdr.Size))
	if err != nil {
		return false, fmt.Errorf("copying %s: %w", abs, err)
	}
	if n < hdr.Size {
		if _, err := io.CopyN(tw, zeros{}, hdr.Size-n); err != nil {
			return false, fmt.Errorf("copying %s: %w", abs, err)
		}
		st.Resized++
	} else if more, _ := f.Read(make([]byte, 1)); more > 0 {
		st.Resized++
	}
	return false, nil
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
