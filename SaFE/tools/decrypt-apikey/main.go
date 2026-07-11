// Command decrypt-apikey recovers the plaintext platform API key ("ak-...")
// from the AES-GCM ciphertext stored in the api_keys.encrypted_key column.
//
// It mirrors common/pkg/apikey/platform_key.go exactly:
//   - AES key      = sha256(cryptoSecret)          (deriveAESKey)
//   - ciphertext   = base64.RawURLEncoding(encoded) (nonce || sealed)
//   - plaintext    = AES-GCM Open, nonce = first NonceSize bytes
//
// IMPORTANT: the crypto secret must be the SAME one the apiserver uses
// (Secret "<release>-crypto", key "key"), otherwise the recovered token is
// garbage and will fail apiserver validation.
//
// The api_keys.api_key column is a one-way HMAC/SHA256 hash and CANNOT be
// decrypted; only encrypted_key is reversible.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	var (
		keyStr  = flag.String("key", "", "crypto secret string (content of the 'key' file in Secret <release>-crypto)")
		keyFile = flag.String("key-file", "", "path to a file containing the crypto secret (alternative to -key)")
		enc     = flag.String("enc", "", "encrypted_key value (base64url) from api_keys.encrypted_key")
		encFile = flag.String("enc-file", "", "path to a file containing the encrypted_key (alternative to -enc)")
	)
	flag.Parse()

	secret, err := loadValue(*keyStr, *keyFile)
	if err != nil {
		fatalf("failed to load crypto key: %v", err)
	}
	ciphertext, err := loadValue(*enc, *encFile)
	if err != nil {
		fatalf("failed to load encrypted_key: %v", err)
	}
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		fatalf("encrypted_key is empty (pass -enc or -enc-file)")
	}

	plaintext, err := decryptPlainToken(ciphertext, []byte(secret))
	if err != nil {
		fatalf("decrypt failed (wrong crypto key or malformed ciphertext?): %v", err)
	}
	fmt.Println(plaintext)
}

// loadValue returns inline when set, else the trimmed file contents.
func loadValue(inline, path string) (string, error) {
	if inline != "" {
		return inline, nil
	}
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// The crypto key file may carry a trailing newline; strip it.
	return strings.TrimRight(string(data), "\r\n"), nil
}

// deriveAESKey matches common/pkg/apikey.deriveAESKey.
func deriveAESKey(secret []byte) []byte {
	hash := sha256.Sum256(secret)
	return hash[:]
}

// decryptPlainToken matches common/pkg/apikey.decryptPlainToken.
func decryptPlainToken(encrypted string, secret []byte) (string, error) {
	key := deriveAESKey(secret)
	data, err := base64.RawURLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := aesGCM.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	return string(plaintext), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
