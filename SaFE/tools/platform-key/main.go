// Command platform-key mirrors common/pkg/apikey.GetOrCreatePlatformKey against
// the api_keys table directly, so an operator can fetch (or mint) a user's
// platform API key ("ak-...") by user id without any user credential.
//
// Behavior (identical to the server):
//   - If an active platform key exists, its encrypted_key is AES-GCM decrypted
//     and the plaintext is printed.
//   - If none exists and --create is set, a new plaintext is generated, hashed
//     (api_key column) and encrypted (encrypted_key column), inserted, and the
//     plaintext printed.
//
// The crypto key MUST be the same one the apiserver uses (Secret
// <release>-crypto, key "key"), otherwise the minted/decrypted token will not
// validate.
//
// WARNING: --create WRITES a row into the production api_keys table.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lib/pq"
)

const (
	tokenPrefix     = "ak-"
	tokenLength     = 32
	keyTypePlatform = "platform"
	platformKeyName = "platform-key"
)

func main() {
	var (
		userID   = flag.String("user-id", "", "target user id (required)")
		userName = flag.String("user-name", "", "target user name (stored on create)")
		create   = flag.Bool("create", false, "mint a new platform key if none exists (WRITES to DB)")

		cryptoKey     = flag.String("crypto-key", "", "crypto secret string")
		cryptoKeyFile = flag.String("crypto-key-file", "", "file containing the crypto secret")

		dbHost     = flag.String("db-host", "", "postgres host (required)")
		dbPort     = flag.String("db-port", "5432", "postgres port")
		dbName     = flag.String("db-name", "", "postgres database name (required)")
		dbUser     = flag.String("db-user", "", "postgres user (required)")
		dbPass     = flag.String("db-password", "", "postgres password")
		dbPassFile = flag.String("db-password-file", "", "file containing the postgres password")
		sslMode    = flag.String("ssl-mode", "require", "postgres sslmode")
	)
	flag.Parse()

	if *userID == "" || *dbHost == "" || *dbName == "" || *dbUser == "" {
		fatalf("required: --user-id, --db-host, --db-name, --db-user")
	}

	secret, err := loadValue(*cryptoKey, *cryptoKeyFile)
	if err != nil {
		fatalf("failed to load crypto key: %v", err)
	}
	password, err := loadValue(*dbPass, *dbPassFile)
	if err != nil {
		fatalf("failed to load db password: %v", err)
	}

	dsn := buildDSN(map[string]string{
		"host":     *dbHost,
		"port":     *dbPort,
		"dbname":   *dbName,
		"user":     *dbUser,
		"password": password,
		"sslmode":  *sslMode,
	})

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	plaintext, minted, err := getOrCreate(db, *userID, *userName, []byte(secret), *create)
	if err != nil {
		fatalf("%v", err)
	}
	if minted {
		fmt.Fprintln(os.Stderr, "note: a new platform key was created and stored")
	}
	fmt.Println(plaintext)
}

// getOrCreate mirrors apikey.GetOrCreatePlatformKey.
func getOrCreate(db *sql.DB, userID, userName string, secret []byte, allowCreate bool) (string, bool, error) {
	enc, found, err := selectEncryptedKey(db, userID)
	if err != nil {
		return "", false, fmt.Errorf("query platform key failed: %w", err)
	}
	if found {
		if enc == "" {
			return "", false, errors.New("platform key row has empty encrypted_key")
		}
		pt, err := decryptPlainToken(enc, secret)
		if err != nil {
			return "", false, fmt.Errorf("decrypt failed (wrong crypto key?): %w", err)
		}
		return pt, false, nil
	}

	if !allowCreate {
		return "", false, errors.New("no active platform key for this user; pass --create to mint one (writes to DB)")
	}

	plain, err := generatePlainToken()
	if err != nil {
		return "", false, err
	}
	hashed := hashPlainToken(plain, secret)
	hint := generateKeyHint(plain)
	encrypted, err := encryptPlainToken(plain, secret)
	if err != nil {
		return "", false, err
	}
	if err := insertPlatformKey(db, userID, userName, hashed, hint, encrypted); err != nil {
		// On unique conflict another writer won the race; re-read and decrypt.
		if isUniqueViolation(err) {
			enc, found, qerr := selectEncryptedKey(db, userID)
			if qerr == nil && found && enc != "" {
				if pt, derr := decryptPlainToken(enc, secret); derr == nil {
					return pt, false, nil
				}
			}
		}
		return "", false, fmt.Errorf("insert platform key failed: %w", err)
	}
	return plain, true, nil
}

func selectEncryptedKey(db *sql.DB, userID string) (string, bool, error) {
	const q = `SELECT encrypted_key FROM api_keys
	           WHERE user_id=$1 AND key_type='platform' AND deleted=false LIMIT 1`
	var enc sql.NullString
	err := db.QueryRow(q, userID).Scan(&enc)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return enc.String, true, nil
}

func insertPlatformKey(db *sql.DB, userID, userName, hashedKey, keyHint, encryptedKey string) error {
	const q = `INSERT INTO api_keys
	           (name, user_id, user_name, api_key, key_hint, expiration_time,
	            creation_time, whitelist, deleted, key_type, encrypted_key)
	           VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`
	farFuture := time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	now := time.Now().UTC()
	_, err := db.Exec(q,
		platformKeyName, userID, userName, hashedKey, keyHint, farFuture,
		now, "[]", false, keyTypePlatform, encryptedKey,
	)
	return err
}

// --- crypto helpers: byte-for-byte identical to common/pkg/apikey ---

func generatePlainToken() (string, error) {
	b := make([]byte, tokenLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func hashPlainToken(plainToken string, secret []byte) string {
	if len(secret) == 0 {
		h := sha256.Sum256([]byte(plainToken))
		return hex.EncodeToString(h[:])
	}
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(plainToken))
	return hex.EncodeToString(h.Sum(nil))
}

func generateKeyHint(plainToken string) string {
	body := strings.TrimPrefix(plainToken, tokenPrefix)
	if len(body) < 6 {
		return tokenPrefix + body
	}
	return tokenPrefix + body[:2] + "****" + body[len(body)-4:]
}

func deriveAESKey(secret []byte) []byte {
	h := sha256.Sum256(secret)
	return h[:]
}

func encryptPlainToken(plaintext string, secret []byte) (string, error) {
	block, err := aes.NewCipher(deriveAESKey(secret))
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := aesGCM.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawURLEncoding.EncodeToString(ct), nil
}

func decryptPlainToken(encrypted string, secret []byte) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decode encrypted key: %w", err)
	}
	block, err := aes.NewCipher(deriveAESKey(secret))
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := aesGCM.NonceSize()
	if len(data) < ns {
		return "", errors.New("ciphertext too short")
	}
	pt, err := aesGCM.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// --- misc ---

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}

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
	return strings.TrimRight(string(data), "\r\n"), nil
}

// buildDSN renders a lib/pq keyword/value DSN with proper escaping.
func buildDSN(kv map[string]string) string {
	var parts []string
	for k, v := range kv {
		if v == "" {
			continue
		}
		escaped := strings.ReplaceAll(v, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `'`, `\'`)
		parts = append(parts, fmt.Sprintf("%s='%s'", k, escaped))
	}
	return strings.Join(parts, " ")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
