/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package log

import (
	"time"
)

type LogInterface interface {
	Request(sinceTime, untilTime time.Time, method, path string, body []byte) ([]byte, error)
	Process(path, method string, body []byte) ([]byte, error)
}
