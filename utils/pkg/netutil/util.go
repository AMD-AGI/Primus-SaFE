/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package netutil

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func GetLocalIp() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", errors.New("failed to find the local ip address")
}

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

func GetHostname(uri string) string {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	return parsedURL.Hostname()
}

func GetSecondLevelDomain(uri string) string {
	hostname := GetHostname(uri)
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
