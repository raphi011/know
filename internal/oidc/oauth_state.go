package oidc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

type OAuthStatePayload struct {
	RedirectURI   string `json:"r"`
	CodeChallenge string `json:"c"`
	ClientState   string `json:"s"`
}

const oauthStatePrefix = "oauth:"

func IsOAuthState(state string) bool {
	return strings.HasPrefix(state, oauthStatePrefix)
}

func SignOAuthState(secret []byte, payload OAuthStatePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal oauth state: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encoded))
	sig := hex.EncodeToString(mac.Sum(nil))
	return oauthStatePrefix + encoded + "." + sig, nil
}

func VerifyOAuthState(secret []byte, state string) (*OAuthStatePayload, error) {
	if !IsOAuthState(state) {
		return nil, fmt.Errorf("not an oauth state")
	}
	state = strings.TrimPrefix(state, oauthStatePrefix)
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed oauth state")
	}
	encoded, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encoded))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, fmt.Errorf("invalid oauth state signature")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode oauth state: %w", err)
	}
	var payload OAuthStatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal oauth state: %w", err)
	}
	return &payload, nil
}
