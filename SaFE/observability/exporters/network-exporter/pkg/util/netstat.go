// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package util

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
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
	defer f.Close()
	tabs, err := parseSocktab(f, fn)
	if err != nil {
		return nil, err
	}
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
		slog.Error("get network interfaces failed", "error", err)
		return result
	}
	for _, iface := range ifaces {
		// Skip interfaces that are not up or have no hardware address
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			slog.Error("get interface address failed", "interface", iface.Name, "error", err)
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

func GetNodeLocalDNS() (string, error) {
	file, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return "", fmt.Errorf("cannot open /etc/resolv.conf: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("load /etc/resolv.conf error: %w", err)
	}
	return "", fmt.Errorf("cannot find nameserver in /etc/resolv.conf")
}

// In checks if a string is in a slice
func In(s string, list []string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
