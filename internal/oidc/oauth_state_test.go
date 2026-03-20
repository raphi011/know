package oidc

import (
	"crypto/rand"
	"testing"
)

func TestSignVerifyOAuthState(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	payload := OAuthStatePayload{
		RedirectURI:   "http://localhost:12345/callback",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		ClientState:   "user-state-abc",
	}
	state, err := SignOAuthState(secret, payload)
	if err != nil {
		t.Fatalf("SignOAuthState: %v", err)
	}
	if state[:6] != "oauth:" {
		t.Errorf("expected oauth: prefix, got %q", state[:6])
	}
	got, err := VerifyOAuthState(secret, state)
	if err != nil {
		t.Fatalf("VerifyOAuthState: %v", err)
	}
	if got.RedirectURI != payload.RedirectURI {
		t.Errorf("redirect_uri: got %q, want %q", got.RedirectURI, payload.RedirectURI)
	}
	if got.CodeChallenge != payload.CodeChallenge {
		t.Errorf("code_challenge: got %q, want %q", got.CodeChallenge, payload.CodeChallenge)
	}
	if got.ClientState != payload.ClientState {
		t.Errorf("client_state: got %q, want %q", got.ClientState, payload.ClientState)
	}
}

func TestVerifyOAuthState_WrongSecret(t *testing.T) {
	secret1 := make([]byte, 32)
	secret2 := make([]byte, 32)
	rand.Read(secret1)
	rand.Read(secret2)
	state, _ := SignOAuthState(secret1, OAuthStatePayload{
		RedirectURI: "http://localhost:1234/callback", CodeChallenge: "challenge", ClientState: "state",
	})
	_, err := VerifyOAuthState(secret2, state)
	if err == nil {
		t.Error("expected error with wrong secret")
	}
}

func TestVerifyOAuthState_Tampered(t *testing.T) {
	secret := make([]byte, 32)
	rand.Read(secret)
	state, _ := SignOAuthState(secret, OAuthStatePayload{
		RedirectURI: "http://localhost:1234/callback", CodeChallenge: "challenge", ClientState: "state",
	})
	tampered := state[:len(state)-2] + "xx"
	_, err := VerifyOAuthState(secret, tampered)
	if err == nil {
		t.Error("expected error with tampered state")
	}
}

func TestIsOAuthState(t *testing.T) {
	if !IsOAuthState("oauth:somebase64.hmac") {
		t.Error("expected true for oauth: prefix")
	}
	if IsOAuthState("ABCDEFGH.hmac") {
		t.Error("expected false for device flow state")
	}
}
