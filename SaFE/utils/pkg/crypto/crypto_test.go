/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package crypto

import (
	"encoding/base64"
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

func TestEncryptInvalidKey(t *testing.T) {
	// AES requires a 16/24/32 byte key, so a short key fails.
	_, err := Encrypt([]byte("msg"), []byte("shortkey"))
	assert.Assert(t, err != nil)
}

func TestDecryptErrors(t *testing.T) {
	validKey := []byte("Arsenal123+_1234")

	// invalid base64 input
	_, err := Decrypt("not base64!!!", validKey)
	assert.Assert(t, err != nil)

	// valid ciphertext but invalid key length
	ciphertext, err := Encrypt([]byte("msg"), validKey)
	assert.NilError(t, err)
	_, err = Decrypt(ciphertext, []byte("short"))
	assert.Assert(t, err != nil)

	// ciphertext shorter than the nonce size
	short := base64.StdEncoding.EncodeToString([]byte("x"))
	_, err = Decrypt(short, validKey)
	assert.Assert(t, err != nil)

	// correct key length but wrong key fails authentication
	_, err = Decrypt(ciphertext, []byte("Brsenal123+_1234"))
	assert.Assert(t, err != nil)
}
