/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package utils

import (
	"os"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
)

func GetDirWatcher(directoryPath string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	err = watcher.Add(directoryPath)
	if err != nil {
		if err2 := watcher.Close(); err2 != nil {
			klog.ErrorS(err2, "fail to close watcher")
		}
		return nil, err
	}
	return watcher, nil
}

func WriteFile(filename, content string, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() {
		if err = f.Close(); err != nil {
			klog.ErrorS(err, "fail to close file")
		}
	}()
	if _, err = f.WriteString(content); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	return nil
}

func IsFileExist(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
