/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// LayerStats describes a written layer.
type LayerStats struct {
	Entries   int
	Whiteouts int
	Bytes     int64
	// DroppedPackages are the dpkg records left out because the package's files are not
	// in the image.
	DroppedPackages []string
}

// tarArgs reads NUL-separated "./"-prefixed names from stdin and archives exactly those
// paths: --no-recursion so a changed directory carries its own metadata and nothing
// else, --no-unquote so a backslash in a name is taken literally, --numeric-owner so
// ownership does not depend on the container's user database. GNU tar exits 1 when a
// file changed or vanished while it was being read, which is expected of a running
// container; anything higher is a failure.
const tarScript = `tar --null --no-unquote --no-recursion --numeric-owner -C / -cf - -T -; rc=$?; ` +
	`if [ "$rc" -gt 1 ]; then exit "$rc"; fi`

// TarInput renders the paths for tarScript's stdin.
func TarInput(paths []string) io.Reader {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(".")
		b.WriteString(p)
		b.WriteByte(0)
	}
	return strings.NewReader(b.String())
}

// WriteLayer copies the archive the container produced into a layer, accepting only the
// paths that were asked for, and appends one OCI whiteout per deleted path. When inImage
// is set, the dpkg status file is reconciled with it (see ReconcileDpkgStatus). It does
// not close w.
func WriteLayer(w io.Writer, archive io.Reader, wanted []string, deleted []string, inImage func(string) bool) (LayerStats, error) {
	want := make(map[string]bool, len(wanted))
	for _, p := range wanted {
		want[p] = true
	}
	var st LayerStats
	cw := &countingWriter{w: w}
	tw := tar.NewWriter(cw)
	tr := tar.NewReader(archive)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return st, fmt.Errorf("reading the container's archive: %w", err)
		}
		abs := path.Clean("/" + hdr.Name)
		if !want[abs] {
			return st, fmt.Errorf("the container's archive holds %q, which was not requested", hdr.Name)
		}
		out := sanitizeHeader(hdr, abs)
		if abs == DpkgStatusPath && inImage != nil && out.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(io.LimitReader(tr, out.Size))
			if err != nil {
				return st, fmt.Errorf("reading %s: %w", abs, err)
			}
			data, st.DroppedPackages = ReconcileDpkgStatus(data, inImage)
			out.Size = int64(len(data))
			if err := tw.WriteHeader(out); err != nil {
				return st, fmt.Errorf("writing %s: %w", abs, err)
			}
			if _, err := tw.Write(data); err != nil {
				return st, fmt.Errorf("writing %s: %w", abs, err)
			}
			st.Entries++
			continue
		}
		if err := tw.WriteHeader(out); err != nil {
			return st, fmt.Errorf("writing %s: %w", abs, err)
		}
		if out.Typeflag == tar.TypeReg && out.Size > 0 {
			if _, err := io.CopyN(tw, tr, out.Size); err != nil {
				return st, fmt.Errorf("copying %s: %w", abs, err)
			}
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

// sanitizeHeader keeps what a layer needs and drops what only describes the machine the
// archive was made on: user and group names (ownership stays numeric), access and change
// times, and any PAX record other than extended attributes.
func sanitizeHeader(in *tar.Header, abs string) *tar.Header {
	out := &tar.Header{
		Typeflag: in.Typeflag,
		Name:     strings.TrimPrefix(abs, "/"),
		Linkname: in.Linkname,
		Size:     in.Size,
		Mode:     in.Mode,
		Uid:      in.Uid,
		Gid:      in.Gid,
		ModTime:  in.ModTime,
		Devmajor: in.Devmajor,
		Devminor: in.Devminor,
		Format:   tar.FormatPAX,
	}
	switch in.Typeflag {
	case tar.TypeDir:
		out.Name += "/"
	case tar.TypeLink:
		// A hard link names another member of the same archive.
		out.Linkname = strings.TrimPrefix(path.Clean("/"+in.Linkname), "/")
	case tar.TypeGNUSparse:
		// The reader has already expanded a sparse file into its full contents.
		out.Typeflag = tar.TypeReg
	}
	for k, v := range in.PAXRecords {
		if strings.HasPrefix(k, "SCHILY.xattr.") {
			if out.PAXRecords == nil {
				out.PAXRecords = map[string]string{}
			}
			out.PAXRecords[k] = v
		}
	}
	return out
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
