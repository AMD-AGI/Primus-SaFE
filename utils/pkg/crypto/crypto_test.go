/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package crypto

import (
	"testing"

	"gotest.tools/assert"
)

func TestCrypto(t *testing.T) {
	key := "Arsenal123+_1234"
	message := "Hello, World!"
	ciphertext, err := Encrypt([]byte(message), []byte(key))
	assert.NilError(t, err)
	decryptedMessage, err := Decrypt(ciphertext, []byte(key))
	assert.NilError(t, err)
	assert.Equal(t, message, string(decryptedMessage))
}
