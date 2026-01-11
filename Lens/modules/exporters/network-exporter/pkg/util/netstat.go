// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package util

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	pathTCPTab  = "/proc/net/tcp"
	pathTCP6Tab = "/proc/net/tcp6"

	ipv4StrLen       = 8
	ipv6StrLen       = 32
	SocketStatListen = 0x0a
)

type SkState uint8

type Socket struct {
	IP   net.IP
	Port uint16
}

type SockTabEntry struct {
	ino        string
	LocalAddr  *Socket
	RemoteAddr *Socket
	State      SkState
	UID        uint32
	Process    *Process
}

type Process struct {
	Pid  int
	Name string
}

func parseIPv4(s string) (net.IP, error) {
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return nil, err
	}
	ip := make(net.IP, net.IPv4len)
	binary.LittleEndian.PutUint32(ip, uint32(v))
	return ip, nil
}

func parseIPv6(s string) (net.IP, error) {
	ip := make(net.IP, net.IPv6len)
	const grpLen = 4
	i, j := 0, 4
	for len(s) != 0 {
		grp := s[0:8]
		u, err := strconv.ParseUint(grp, 16, 32)
		binary.LittleEndian.PutUint32(ip[i:j], uint32(u))
		if err != nil {
			return nil, err
		}
		i, j = i+grpLen, j+grpLen
		s = s[8:]
	}
	return ip, nil
}

func parseAddr(s string) (*Socket, error) {
	fields := strings.Split(s, ":")
	if len(fields) < 2 {
		return nil, fmt.Errorf("netstat: not enough fields: %v", s)
	}
	var ip net.IP
	var err error
	switch len(fields[0]) {
	case ipv4StrLen:
		ip, err = parseIPv4(fields[0])
	case ipv6StrLen:
		ip, err = parseIPv6(fields[0])
	default:
		err = fmt.Errorf("netstat: bad formatted string: %v", fields[0])
	}
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseUint(fields[1], 16, 16)
	if err != nil {
		return nil, err
	}
	return &Socket{IP: ip, Port: uint16(v)}, nil
}

type AcceptFn func(*SockTabEntry) bool

func doNetstat(path string, fn AcceptFn) ([]SockTabEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	tabs, err := parseSocktab(f, fn)
	f.Close()
	if err != nil {
		return nil, err
	}
	extractProcInfo(tabs)
	return tabs, nil
}

func parseSocktab(r io.Reader, accept AcceptFn) ([]SockTabEntry, error) {
	br := bufio.NewScanner(r)
	tab := make([]SockTabEntry, 0, 4)

	// Discard title
	br.Scan()

	for br.Scan() {
		var e SockTabEntry
		line := br.Text()
		// Skip comments
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 12 {
			return nil, fmt.Errorf("netstat: not enough fields: %v, %v", len(fields), fields)
		}
		addr, err := parseAddr(fields[1])
		if err != nil {
			return nil, err
		}
		e.LocalAddr = addr
		addr, err = parseAddr(fields[2])
		if err != nil {
			return nil, err
		}
		e.RemoteAddr = addr
		u, err := strconv.ParseUint(fields[3], 16, 8)
		if err != nil {
			return nil, err
		}
		e.State = SkState(u)
		u, err = strconv.ParseUint(fields[7], 10, 32)
		if err != nil {
			return nil, err
		}
		e.UID = uint32(u)
		e.ino = fields[9]
		if accept(&e) {
			tab = append(tab, e)
		}
	}
	return tab, br.Err()
}

func extractProcInfo(sktab []SockTabEntry) {
	const basedir = "/proc"
	fi, err := ioutil.ReadDir(basedir)
	if err != nil {
		return
	}

	for _, file := range fi {
		if !file.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(file.Name())
		if err != nil {
			continue
		}
		base := path.Join(basedir, file.Name())
		proc := procFd{base: base, pid: pid, sktab: sktab}
		proc.iterFdDir()
	}
}

func (p *procFd) iterFdDir() {
	fddir := path.Join(p.base, "/fd")
	fi, err := os.ReadDir(fddir)
	if err != nil {
		return
	}
	var buf [128]byte

	for _, file := range fi {
		fd := path.Join(fddir, file.Name())
		lname, err := os.Readlink(fd)
		if err != nil || !strings.HasPrefix(lname, sockPrefix) {
			continue
		}

		for i := range p.sktab {
			sk := &p.sktab[i]
			ss := sockPrefix + sk.ino + "]"
			if ss != lname {
				continue
			}
			if p.p == nil {
				stat, err := os.Open(path.Join(p.base, "stat"))
				if err != nil {
					return
				}
				n, err := stat.Read(buf[:])
				stat.Close()
				if err != nil {
					return
				}
				z := bytes.SplitN(buf[:n], []byte(" "), 3)
				name := getProcName(z[1])
				p.p = &Process{p.pid, name}
			}
			sk.Process = p.p
		}
	}
}

type procFd struct {
	base  string
	pid   int
	sktab []SockTabEntry
	p     *Process
}

const sockPrefix = "socket:["

func getProcName(s []byte) string {
	i := bytes.Index(s, []byte("("))
	if i < 0 {
		return ""
	}
	j := bytes.LastIndex(s, []byte(")"))
	if i > j {
		return ""
	}
	return string(s[i+1 : j])
}

func TCPSocks(accept AcceptFn) ([]SockTabEntry, error) {
	return doNetstat(pathTCPTab, accept)
}

func TcpListen() ([]SockTabEntry, error) {
	return TcpListenWithCustomPath(pathTCPTab)
}

func TcpListenWithCustomPath(path string) ([]SockTabEntry, error) {
	return doNetstat(path, func(entry *SockTabEntry) bool {
		return entry.State == SocketStatListen
	})
}

func GetLocalIpV4() []string {
	result := make([]string, 0)
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Errorf("get network interfaces failed %s", err)
		return result
	}
	for _, iface := range ifaces {
		// Skip interfaces that are not up or have no hardware address
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			log.Errorf("get interface %s address failed %s", iface.Name, err)
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			result = append(result, ip.String())
		}
	}
	return result
}
