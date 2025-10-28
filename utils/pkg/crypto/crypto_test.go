/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package crypto

import (
	"testing"

	"gotest.tools/assert"
)

func TestCrypto(t *testing.T) {
	key := "Arsenal123+_1234"
	message := "weilei-1756370912"
	ciphertext, err := Encrypt([]byte(message), []byte(key))
	assert.NilError(t, err)

	decryptedMessage, err := Decrypt(ciphertext, []byte(key))
	assert.NilError(t, err)
	assert.Equal(t, message, string(decryptedMessage))
}
