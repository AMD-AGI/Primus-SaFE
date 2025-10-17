/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type GetEnvResponse struct {
	// Whether to enable log download.
	EnableLogDownload bool `json:"enableLogDownload"`
	// Whether to enable the entire logging functionality, including log download.
	EnableLog bool `json:"enableLog"`
	// Whether to enable ssh include webshell.
	EnableSSH bool `json:"enableSsh"`
	// The image used for authoring.
	AuthoringImage string `json:"authoringImage"`
	// The port for ssh
	SSHPort int `json:"sshPort"`
	// The ip for ssh
	SSHIP string `json:"sshIP"`
}
