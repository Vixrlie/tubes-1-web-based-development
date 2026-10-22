package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func randomString(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewToken membuat session token: 32 byte acak dari crypto/rand, base64url.
func NewToken() (string, error) {
	return randomString(32)
}

// HashToken adalah satu-satunya bentuk token yang disimpan di database.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
