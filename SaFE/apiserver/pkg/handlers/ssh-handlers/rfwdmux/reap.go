/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package rfwdmux

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// reapWait is how long to wait for a signaled multiplexer to exit and drop the
// port. A listen socket is released when that process closes it, which it does
// before exiting.
const reapWait = 5 * time.Second

// ReapStaleListener signals a multiplexer that is still listening on addr:port
// in this network namespace. A process whose command is not `mux listen` for
// that address is left alone. It reports whether it signaled one.
func ReapStaleListener(addr string, port uint16) (bool, error) {
	pids, err := muxListenerPIDs(addr, port)
	if err != nil {
		return false, err
	}
	signaled := false
	for _, pid := range pids {
		if pid <= 1 || pid == os.Getpid() {
			continue
		}
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			if err == syscall.ESRCH {
				continue
			}
			return signaled, fmt.Errorf("signal multiplexer %d: %v", pid, err)
		}
		signaled = true
		if err := waitExit(pid, reapWait); err != nil {
			return signaled, err
		}
	}
	return signaled, nil
}

// muxListenerPIDs returns processes listening on addr:port whose command line is
// a mux listen for that same address.
func muxListenerPIDs(addr string, port uint16) ([]int, error) {
	inodes, err := listenInodes(addr, port)
	if err != nil || len(inodes) == 0 {
		return nil, err
	}
	procDirs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %v", err)
	}
	var pids []int
	for _, dir := range procDirs {
		pid, err := strconv.Atoi(dir.Name())
		if err != nil {
			continue
		}
		if !holdsSocket(pid, inodes) {
			continue
		}
		if !isMuxListen(cmdlineArgs(pid), addr, port) {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// listenInodes returns the socket inodes of IPv4 listeners on addr:port.
func listenInodes(addr string, port uint16) (map[string]struct{}, error) {
	ip := net.ParseIP(addr).To4()
	if ip == nil {
		return nil, fmt.Errorf("%s is not an IPv4 address", addr)
	}
	want := fmt.Sprintf("%02X%02X%02X%02X:%04X", ip[3], ip[2], ip[1], ip[0], port)
	file, err := os.Open("/proc/net/tcp")
	if err != nil {
		return nil, fmt.Errorf("read /proc/net/tcp: %v", err)
	}
	defer file.Close()

	inodes := map[string]struct{}{}
	scanner := bufio.NewScanner(file)
	header := true
	for scanner.Scan() {
		if header {
			header = false
			continue
		}
		fields := strings.Fields(scanner.Text())
		// local address, state, inode: see the kernel's tcp4_seq_show.
		if len(fields) < 10 || fields[3] != "0A" || !strings.EqualFold(fields[1], want) {
			continue
		}
		inodes[fields[9]] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read /proc/net/tcp: %v", err)
	}
	return inodes, nil
}

// holdsSocket reports whether pid has a socket inode from the set open.
func holdsSocket(pid int, inodes map[string]struct{}) bool {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
		if err != nil {
			continue
		}
		inode, ok := strings.CutPrefix(target, "socket:[")
		if !ok || !strings.HasSuffix(inode, "]") {
			continue
		}
		inode = strings.TrimSuffix(inode, "]")
		if _, wanted := inodes[inode]; wanted {
			return true
		}
	}
	return false
}

// cmdlineArgs reads the NUL-separated argument vector of pid.
func cmdlineArgs(pid int) []string {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return nil
	}
	parts := bytes.Split(raw, []byte{0})
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		args = append(args, string(part))
	}
	return args
}

// isMuxListen reports whether args is `mux listen ... <addr> <port>`.
func isMuxListen(args []string, addr string, port uint16) bool {
	if len(args) < 4 || filepath.Base(args[0]) != "mux" || args[1] != "listen" {
		return false
	}
	return args[len(args)-2] == addr && args[len(args)-1] == strconv.Itoa(int(port))
}

// waitExit waits until pid has exited. A zombie counts: it has already closed
// its listen socket, and in a test the parent may not have reaped it yet.
func waitExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("multiplexer %d did not exit after %s", pid, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// processAlive reports whether pid is still running. Zombies and missing
// processes are not alive for this purpose.
func processAlive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// comm is wrapped in parentheses and may itself contain spaces; the state
	// character is the first field after the closing parenthesis.
	text := string(stat)
	end := strings.LastIndex(text, ")")
	if end < 0 || end+2 >= len(text) {
		return true
	}
	state := text[end+2]
	return state != 'Z' && state != 'X'
}
