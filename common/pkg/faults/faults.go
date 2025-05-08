/*
   Copyright © 01.AI Co., Ltd. 2023-2024. All rights reserved.
*/

package faults

import (
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
)

func GenerateFaultName(adminNodeName, code string) string {
	name := adminNodeName + "-" + code
	return stringutil.NormalizeName(name)
}
