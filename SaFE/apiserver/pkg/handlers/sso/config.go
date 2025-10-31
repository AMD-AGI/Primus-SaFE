/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package sso

import (
	"fmt"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

type config struct {
	endpoint string
	id       string
	secret   string
}

func initConfig() (*config, error) {
	c := &config{
		endpoint: commonconfig.GetSSOEndpoint(),
		id:       commonconfig.GetSSOClientId(),
		secret:   commonconfig.GetSSOClientSecret(),
	}
	if c.endpoint == "" || c.id == "" || c.secret == "" {
		return nil, fmt.Errorf("failed to find sso config")
	}
	return c, nil
}
