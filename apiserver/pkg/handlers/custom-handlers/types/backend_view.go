/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

type GetEnvResponse struct {
	EnableLogDownload bool   `json:"enableLogDownload"`
	EnableLog         bool   `json:"enableLog"`
	EnableSSH         bool   `json:"enableSsh"`
	AuthoringImage    string `json:"authoringImage"`
	SSHPort           int  `json:"sshPort"`
}
