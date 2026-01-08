// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package rest

type Page struct {
	PageNum  int `json:"page_num" form:"page_num,default=1"`
	PageSize int `json:"page_size" form:"page_size,default=10"`
}
