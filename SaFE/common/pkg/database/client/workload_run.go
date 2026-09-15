/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import "fmt"

// listWorkloadRunSQL selects pod or dispatch rows of one CR generation. Rows
// written before workload_uid existed (empty uid) are returned only when this
// generation has no rows of its own.
func listWorkloadRunSQL(table, orderBy string) string {
	return fmt.Sprintf(`SELECT * FROM %s WHERE workload_id = $1 AND (
		workload_uid = $2 OR (
			$2 <> '' AND workload_uid = '' AND NOT EXISTS (
				SELECT 1 FROM %s cur WHERE cur.workload_id = $1 AND cur.workload_uid = $2
			)
		)
	) ORDER BY %s`, table, table, orderBy)
}
