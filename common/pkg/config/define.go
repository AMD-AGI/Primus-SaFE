/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

const (
	// global
	globalPrefix = "global."
	cryptoEnable = globalPrefix + "enable_crypto"

	// server
	serverPrefix = "server."
	serverPort   = serverPrefix + "port"

	// health_check
	healthCheckPrefix = "health_check."
	healthCheckEnable = healthCheckPrefix + "enable"
	healthCheckPort   = healthCheckPrefix + "port"

	// leader_election
	leaderElectionPrefix = "leader_election."
	leaderElectionEnable = leaderElectionPrefix + "enable"
	leaderElectionLock   = leaderElectionPrefix + "lock_namespace"
)
