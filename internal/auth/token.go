package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const tokenPrefix = "kh_"

// GenerateToken creates a new API token: kh_<64 hex chars>.
// Returns the raw token (shown once) and its hash (stored in DB).
func GenerateToken() (raw string, hash string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}
	raw = tokenPrefix + hex.EncodeToString(bytes)
	hash = HashToken(raw)
	return raw, hash, nil
}

// HashToken computes SHA256 of a raw token string.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
