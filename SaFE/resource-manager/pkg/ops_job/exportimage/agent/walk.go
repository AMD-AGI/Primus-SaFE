/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Walk lists the file system rooted at root, as the image sees it: paths are absolute
// within root ("/usr/bin/python3"), excluded paths are left out and not descended into,
// and no other file system is entered. A file that disappears during the walk is skipped;
// any other error fails the walk, because a directory that could not be read would look
// like one whose files were all deleted.
func Walk(root string, f Filter) ([]Entry, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	rootDev, ok := deviceOf(rootInfo)
	if !ok {
		return nil, fmt.Errorf("cannot read the device of %s", root)
	}
	var out []Entry
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		abs, err := absPath(root, p)
		if err != nil {
			return err
		}
		if abs != "/" && f.Excluded(abs) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := os.Lstat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("cannot read the status of %s", abs)
		}
		if info.IsDir() && uint64(st.Dev) != rootDev {
			// Another file system mounted here (-xdev): neither it nor its contents are
			// part of the image.
			return filepath.SkipDir
		}
		out = append(out, Entry{
			Path:  abs,
			Type:  typeLetter(info.Mode()),
			Ctime: Timestamp{Sec: int64(st.Ctim.Sec), Nsec: int64(st.Ctim.Nsec)},
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing the container's files: %w", err)
	}
	return out, nil
}

func absPath(root, p string) (string, error) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return "/", nil
	}
	return "/" + filepath.ToSlash(rel), nil
}

func deviceOf(info fs.FileInfo) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}

func typeLetter(m fs.FileMode) byte {
	switch {
	case m.IsDir():
		return 'd'
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m&fs.ModeSocket != 0:
		return 's'
	case m&fs.ModeNamedPipe != 0:
		return 'p'
	case m&fs.ModeCharDevice != 0:
		return 'c'
	case m&fs.ModeDevice != 0:
		return 'b'
	}
	return 'f'
}

func ctimeOf(info fs.FileInfo) (Timestamp, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return Timestamp{}, false
	}
	return Timestamp{Sec: int64(st.Ctim.Sec), Nsec: int64(st.Ctim.Nsec)}, true
}
