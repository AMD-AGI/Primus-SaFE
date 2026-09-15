/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import "fmt"

// listWorkloadRunSQL selects pod or dispatch rows of one CR generation.
func listWorkloadRunSQL(table, orderBy string) string {
	return fmt.Sprintf(
		`SELECT * FROM %s WHERE workload_id = $1 AND workload_uid = $2 ORDER BY %s`,
		table, orderBy)
}
