// Package keychain stores and retrieves credentials from the OS-native
// credential store (macOS Keychain, Linux libsecret, Windows Credential Manager).
package keychain

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "know"

// ErrNotFound is returned when no credential is stored for the requested key.
var ErrNotFound = keyring.ErrNotFound

// SetToken stores the API token in the system keychain.
func SetToken(token string) error {
	if err := keyring.Set(service, "token", token); err != nil {
		return fmt.Errorf("set token in keychain: %w", err)
	}
	return nil
}

// GetToken retrieves the API token from the system keychain.
// Returns ErrNotFound if no token is stored.
func GetToken() (string, error) {
	token, err := keyring.Get(service, "token")
	if err != nil {
		return "", fmt.Errorf("get token from keychain: %w", err)
	}
	return token, nil
}

// DeleteToken removes the API token from the system keychain.
func DeleteToken() error {
	if err := keyring.Delete(service, "token"); err != nil {
		return fmt.Errorf("delete token from keychain: %w", err)
	}
	return nil
}

// SetAPIURL stores the server URL in the system keychain.
func SetAPIURL(url string) error {
	if err := keyring.Set(service, "api_url", url); err != nil {
		return fmt.Errorf("set api url in keychain: %w", err)
	}
	return nil
}

// GetAPIURL retrieves the server URL from the system keychain.
// Returns ErrNotFound if no URL is stored.
func GetAPIURL() (string, error) {
	url, err := keyring.Get(service, "api_url")
	if err != nil {
		return "", fmt.Errorf("get api url from keychain: %w", err)
	}
	return url, nil
}

// DeleteAPIURL removes the server URL from the system keychain.
func DeleteAPIURL() error {
	if err := keyring.Delete(service, "api_url"); err != nil {
		return fmt.Errorf("delete api url from keychain: %w", err)
	}
	return nil
}

// IsNotFound reports whether the error indicates that no credential was found.
func IsNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}
