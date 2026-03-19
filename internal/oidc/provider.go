package oidc

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Provider abstracts an OAuth2/OIDC identity provider.
// Only AuthCodeURL and ExchangeCode differ between providers;
// device flow, PKCE, and user resolution are provider-agnostic.
type Provider interface {
	// ProviderName returns a stable string used as the DB key for
	// this identity provider (e.g. "github", "google").
	ProviderName() string

	// AuthCodeURL returns the URL to redirect the user to for authorization.
	AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string

	// ExchangeCode exchanges an authorization code for user info.
	ExchangeCode(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*UserInfo, error)
}

// Compile-time interface satisfaction checks.
var (
	_ Provider = (*OIDCProvider)(nil)
	_ Provider = (*GitHubProvider)(nil)
)

// OIDCProvider implements Provider using standard OIDC discovery and ID token verification.
type OIDCProvider struct {
	oidcProvider *gooidc.Provider
	oauth2Config oauth2.Config
	issuerURL    string
	providerName string // explicit name for DB key; falls back to URL-derived
}

// UserInfo holds the claims extracted from an OIDC token.
type UserInfo struct {
	Subject  string // unique ID from provider
	Email    string
	Name     string
	Provider string // e.g. "github"
}

// NewOIDC creates a new OIDC provider by discovering the issuer's configuration.
// providerName is used as the DB key for user identity. If empty, it is derived from the issuer URL.
func NewOIDC(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string, scopes []string, providerName string) (*OIDCProvider, error) {
	oidcProvider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover oidc provider: %w", err)
	}

	if len(scopes) == 0 {
		scopes = []string{gooidc.ScopeOpenID, "profile", "email"}
	}

	return &OIDCProvider{
		oidcProvider: oidcProvider,
		oauth2Config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     oidcProvider.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       scopes,
		},
		issuerURL:    issuerURL,
		providerName: providerName,
	}, nil
}

// ProviderName returns the provider name used as a DB key for OIDC identity.
// If an explicit name was set via New(), it is returned directly.
// Otherwise, a short name is derived from the issuer URL:
// e.g. "https://github.com" -> "github", "https://accounts.google.com" -> "google"
func (p *OIDCProvider) ProviderName() string {
	if p.providerName != "" {
		return p.providerName
	}
	u, err := url.Parse(p.issuerURL)
	if err != nil {
		return p.issuerURL
	}
	host := strings.TrimPrefix(u.Hostname(), "www.")
	host = strings.TrimPrefix(host, "accounts.")
	if dot := strings.IndexByte(host, '.'); dot > 0 {
		return host[:dot]
	}
	return host
}

// ExchangeCode exchanges an authorization code for user info.
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*UserInfo, error) {
	token, err := p.oauth2Config.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	verifier := p.oidcProvider.Verifier(&gooidc.Config{ClientID: p.oauth2Config.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}

	var claims struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Login string `json:"login"` // GitHub-specific
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	if idToken.Subject == "" {
		return nil, fmt.Errorf("id token has empty subject")
	}

	name := claims.Name
	if name == "" {
		name = claims.Login // fallback for GitHub
	}

	return &UserInfo{
		Subject:  idToken.Subject,
		Email:    claims.Email,
		Name:     name,
		Provider: p.ProviderName(),
	}, nil
}

// AuthCodeURL returns the URL to redirect the user to for authorization.
func (p *OIDCProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return p.oauth2Config.AuthCodeURL(state, opts...)
}
