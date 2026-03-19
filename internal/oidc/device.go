package oidc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// GenerateDeviceCode creates a new device authorization code pair.
// userCode is 8 uppercase letters (XXXX-XXXX format for display), deviceCode is 64 hex chars.
func GenerateDeviceCode() (userCode, deviceCode string, err error) {
	// Generate user code: 8 random uppercase letters
	uc := make([]byte, 8)
	if _, err := rand.Read(uc); err != nil {
		return "", "", fmt.Errorf("generate user code: %w", err)
	}
	for i := range uc {
		uc[i] = 'A' + (uc[i] % 26)
	}
	userCode = string(uc)

	// Generate device code: 32 random bytes → 64 hex chars
	dc := make([]byte, 32)
	if _, err := rand.Read(dc); err != nil {
		return "", "", fmt.Errorf("generate device code: %w", err)
	}
	deviceCode = hex.EncodeToString(dc)

	return userCode, deviceCode, nil
}

// FormatUserCode formats a user code for display (e.g. "ABCD-EFGH").
func FormatUserCode(code string) string {
	if len(code) >= 8 {
		return strings.ToUpper(code[:4]) + "-" + strings.ToUpper(code[4:8])
	}
	if len(code) > 4 {
		return strings.ToUpper(code[:4]) + "-" + strings.ToUpper(code[4:])
	}
	return strings.ToUpper(code)
}

// SignState creates an HMAC-signed state parameter for the OIDC redirect.
// The state encodes the user_code so the callback can approve the device flow.
func SignState(secret []byte, userCode string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(userCode))
	sig := hex.EncodeToString(mac.Sum(nil))
	return userCode + "." + sig
}

// VerifyState verifies an HMAC-signed state parameter and returns the user_code.
func VerifyState(secret []byte, state string) (string, bool) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	userCode := parts[0]
	expected := SignState(secret, userCode)
	if !hmac.Equal([]byte(state), []byte(expected)) {
		return "", false
	}
	return userCode, true
}
