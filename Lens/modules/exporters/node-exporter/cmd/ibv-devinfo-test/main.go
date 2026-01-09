// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Port struct {
	PortNum       int
	State         string
	MaxMTU        int
	ActiveMTU     int
	LinkLayer     string
	ActiveWidth   string
	ActiveSpeed   string
	PhysState     string
	GIDs          []string
	PortCapFlags  string
	PortCapFlags2 string
}

type HCA struct {
	HcaID          string
	Transport      string
	FwVer          string
	NodeGUID       string
	SysImageGUID   string
	VendorID       string
	VendorPartID   string
	HWVer          string
	PhysPortCount  int
	MaxMRSize      string
	MaxQP          int
	DeviceCapFlags string
	NumCompVectors int
	Ports          []Port
}

func parseIBVDevinfo(file string) ([]HCA, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hcas []HCA
	var currentHCA *HCA
	var currentPort *Port

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "hca_id:") {
			if currentHCA != nil {
				if currentPort != nil {
					currentHCA.Ports = append(currentHCA.Ports, *currentPort)
					currentPort = nil
				}
				hcas = append(hcas, *currentHCA)
			}
			currentHCA = &HCA{
				HcaID: strings.TrimSpace(strings.TrimPrefix(line, "hca_id:")),
			}
			continue
		}

		if strings.HasPrefix(line, "transport:") {
			currentHCA.Transport = strings.TrimSpace(strings.TrimPrefix(line, "transport:"))
		} else if strings.HasPrefix(line, "fw_ver:") {
			currentHCA.FwVer = strings.TrimSpace(strings.TrimPrefix(line, "fw_ver:"))
		} else if strings.HasPrefix(line, "node_guid:") {
			currentHCA.NodeGUID = strings.TrimSpace(strings.TrimPrefix(line, "node_guid:"))
		} else if strings.HasPrefix(line, "sys_image_guid:") {
			currentHCA.SysImageGUID = strings.TrimSpace(strings.TrimPrefix(line, "sys_image_guid:"))
		} else if strings.HasPrefix(line, "vendor_id:") {
			currentHCA.VendorID = strings.TrimSpace(strings.TrimPrefix(line, "vendor_id:"))
		} else if strings.HasPrefix(line, "vendor_part_id:") {
			currentHCA.VendorPartID = strings.TrimSpace(strings.TrimPrefix(line, "vendor_part_id:"))
		} else if strings.HasPrefix(line, "hw_ver:") {
			currentHCA.HWVer = strings.TrimSpace(strings.TrimPrefix(line, "hw_ver:"))
		} else if strings.HasPrefix(line, "phys_port_cnt:") {
			v, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "phys_port_cnt:")))
			currentHCA.PhysPortCount = v
		} else if strings.HasPrefix(line, "max_mr_size:") {
			currentHCA.MaxMRSize = strings.TrimSpace(strings.TrimPrefix(line, "max_mr_size:"))
		} else if strings.HasPrefix(line, "max_qp:") {
			v, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "max_qp:")))
			currentHCA.MaxQP = v
		} else if strings.HasPrefix(line, "device_cap_flags:") {
			currentHCA.DeviceCapFlags = strings.TrimSpace(strings.TrimPrefix(line, "device_cap_flags:"))
		} else if strings.HasPrefix(line, "num_comp_vectors:") {
			v, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "num_comp_vectors:")))
			currentHCA.NumCompVectors = v
		} else if strings.HasPrefix(line, "port:") {
			if currentPort != nil {
				currentHCA.Ports = append(currentHCA.Ports, *currentPort)
			}
			portNum, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "port:")))
			currentPort = &Port{PortNum: portNum}
		} else if strings.HasPrefix(line, "state:") {
			currentPort.State = strings.TrimSpace(strings.TrimPrefix(line, "state:"))
		} else if strings.HasPrefix(line, "max_mtu:") {
			mtu, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "max_mtu:")))
			currentPort.MaxMTU = mtu
		} else if strings.HasPrefix(line, "active_mtu:") {
			mtu, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "active_mtu:")))
			currentPort.ActiveMTU = mtu
		} else if strings.HasPrefix(line, "link_layer:") {
			currentPort.LinkLayer = strings.TrimSpace(strings.TrimPrefix(line, "link_layer:"))
		} else if strings.HasPrefix(line, "active_width:") {
			currentPort.ActiveWidth = strings.TrimSpace(strings.TrimPrefix(line, "active_width:"))
		} else if strings.HasPrefix(line, "active_speed:") {
			currentPort.ActiveSpeed = strings.TrimSpace(strings.TrimPrefix(line, "active_speed:"))
		} else if strings.HasPrefix(line, "phys_state:") {
			currentPort.PhysState = strings.TrimSpace(strings.TrimPrefix(line, "phys_state:"))
		} else if strings.HasPrefix(line, "GID[") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				currentPort.GIDs = append(currentPort.GIDs, strings.TrimSpace(parts[1]))
			}
		} else if strings.HasPrefix(line, "port_cap_flags:") {
			currentPort.PortCapFlags = strings.TrimSpace(strings.TrimPrefix(line, "port_cap_flags:"))
		} else if strings.HasPrefix(line, "port_cap_flags2:") {
			currentPort.PortCapFlags2 = strings.TrimSpace(strings.TrimPrefix(line, "port_cap_flags2:"))
		}
	}

	if currentHCA != nil {
		if currentPort != nil {
			currentHCA.Ports = append(currentHCA.Ports, *currentPort)
		}
		hcas = append(hcas, *currentHCA)
	}

	return hcas, scanner.Err()
}

func main() {
	hcas, err := parseIBVDevinfo("ibv_devinfo.txt")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	for _, hca := range hcas {
		fmt.Printf("%+v\n", hca)
	}
}
