package oidc

import (
	"testing"
)

func TestGenerateDeviceCode(t *testing.T) {
	userCode, deviceCode, err := GenerateDeviceCode()
	if err != nil {
		t.Fatalf("GenerateDeviceCode failed: %v", err)
	}

	if len(userCode) != 8 {
		t.Errorf("expected user code length 8, got %d: %q", len(userCode), userCode)
	}
	for _, c := range userCode {
		if c < 'A' || c > 'Z' {
			t.Errorf("user code contains non-uppercase char: %q", userCode)
			break
		}
	}

	if len(deviceCode) != 64 {
		t.Errorf("expected device code length 64, got %d: %q", len(deviceCode), deviceCode)
	}

	// Uniqueness: two calls should produce different codes
	userCode2, deviceCode2, err := GenerateDeviceCode()
	if err != nil {
		t.Fatalf("second GenerateDeviceCode failed: %v", err)
	}
	if userCode == userCode2 {
		t.Error("two user codes should not be identical")
	}
	if deviceCode == deviceCode2 {
		t.Error("two device codes should not be identical")
	}
}

func TestFormatUserCode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ABCDEFGH", "ABCD-EFGH"},
		{"abcdefgh", "ABCD-EFGH"},
		{"ABCD", "ABCD"},
		{"ABCDE", "ABCD-E"},
		{"AB", "AB"},
	}
	for _, tt := range tests {
		got := FormatUserCode(tt.input)
		if got != tt.want {
			t.Errorf("FormatUserCode(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSignAndVerifyState(t *testing.T) {
	secret := []byte("test-secret-key-32bytes!12345678")
	userCode := "ABCDEFGH"

	state := SignState(secret, userCode)

	// Should verify successfully
	got, ok := VerifyState(secret, state)
	if !ok {
		t.Fatal("VerifyState returned false for valid state")
	}
	if got != userCode {
		t.Errorf("VerifyState returned %q, want %q", got, userCode)
	}

	// Wrong secret should fail
	_, ok = VerifyState([]byte("wrong-secret-key-32bytes!1234567"), state)
	if ok {
		t.Error("VerifyState should fail with wrong secret")
	}

	// Tampered state should fail
	_, ok = VerifyState(secret, state+"x")
	if ok {
		t.Error("VerifyState should fail with tampered state")
	}

	// No dot separator
	_, ok = VerifyState(secret, "nodot")
	if ok {
		t.Error("VerifyState should fail without dot separator")
	}

	// Empty state
	_, ok = VerifyState(secret, "")
	if ok {
		t.Error("VerifyState should fail on empty state")
	}
}
