/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package netutil

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// GetLocalIp returns the local IP address of the machine.
func GetLocalIp() (string, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addresses {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", errors.New("failed to find the local ip address")
}

// ConvertIpToInt converts data to the target format.
func ConvertIpToInt(ip string) int {
	values := strings.Split(ip, ".")
	valuesInt := make([]int, len(values))
	var err error
	for i := range values {
		valuesInt[i], err = strconv.Atoi(values[i])
		if err != nil {
			return 0
		}
	}
	return (valuesInt[0] << 24) + (valuesInt[1] << 16) + (valuesInt[2] << 8) + valuesInt[3]
}

// GetHostname returns the hostname of the node.
func GetHostname(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// GetSchemeHost extracts and returns the scheme and host portion of a URL.
func GetSchemeHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// GetSecondLevelDomain extracts and returns the second-level domain from a given URI.
func GetSecondLevelDomain(uri string) string {
	hostname := GetHostname(uri)
	if hostname == "" {
		hostname = uri
	}
	if hostname == "127.0.0.1" || hostname == "localhost" {
		return hostname
	}
	parts := strings.Split(hostname, ".")
	if len(parts) <= 2 {
		return hostname
	}
	secondLevelDomain := parts[len(parts)-2] + "." + parts[len(parts)-1]
	return secondLevelDomain
}
