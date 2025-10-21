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
// It iterates through all network interfaces and returns the first non-loopback IPv4 address found.
// Returns an error if no valid local IP address is found.
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

// ConvertIpToInt converts an IPv4 address string to its integer representation.
// It splits the IP by dots, converts each part to integer, and combines them using bit shifting.
// Returns 0 if any part of the IP cannot be converted to integer.
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

// GetHostname extracts and returns the hostname from a given URI.
// It parses the URI using url.Parse and returns the hostname component.
// Returns an empty string if the URI cannot be parsed.
func GetHostname(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// GetSchemeHost extracts and returns the scheme and host portion of a URL.
// It parses the URL and returns a string in the format "scheme://host".
// Returns an empty string if the URL cannot be parsed or if scheme/host is missing.
func GetSchemeHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// GetSecondLevelDomain extracts and returns the second-level domain from a given URI.
// For example, for "www.example.com", it returns "example.com".
// Special cases like "127.0.0.1" and "localhost" are returned as-is.
// Returns the original URI if it cannot be parsed or if it's already a short domain.
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
