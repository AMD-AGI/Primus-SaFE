/*
 * Copyright © AMD. 2025-2026. All rights reserved.
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
