/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"encoding/hex"
	"encoding/json"
	"strconv"
)

// timeToHex formats a unix-nano timestamp as a compact lowercase hex string.
func timeToHex(n int64) string {
	return strconv.FormatInt(n, 16)
}

// seqToHex formats a sequence number as a zero-padded 6-char hex suffix.
func seqToHex(n uint64) string {
	buf := make([]byte, 3)
	buf[0] = byte(n >> 16)
	buf[1] = byte(n >> 8)
	buf[2] = byte(n)
	return hex.EncodeToString(buf)
}

// marshalPayload serializes an event payload into json.RawMessage without
// losing the type tag. Returns "null" on marshalling error so the client
// never sees an invalid envelope.
func marshalPayload(v interface{}) json.RawMessage {
	if v == nil {
		return json.RawMessage(`null`)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`null`)
	}
	return data
}
