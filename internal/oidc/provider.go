package oidc

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Provider wraps an OIDC provider with OAuth2 configuration.
type Provider struct {
	oidcProvider *gooidc.Provider
	oauth2Config oauth2.Config
	issuerURL    string
}

// UserInfo holds the claims extracted from an OIDC token.
type UserInfo struct {
	Subject  string // unique ID from provider
	Email    string
	Name     string
	Provider string // e.g. "github"
}

// New creates a new OIDC provider by discovering the issuer's configuration.
func New(ctx context.Context, issuerURL, clientID, clientSecret, redirectURL string, scopes []string) (*Provider, error) {
	oidcProvider, err := gooidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover oidc provider: %w", err)
	}

	if len(scopes) == 0 {
		scopes = []string{gooidc.ScopeOpenID, "profile", "email"}
	}

	return &Provider{
		oidcProvider: oidcProvider,
		oauth2Config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     oidcProvider.Endpoint(),
			RedirectURL:  redirectURL,
			Scopes:       scopes,
		},
		issuerURL: issuerURL,
	}, nil
}

// ProviderName extracts a short provider name from the issuer URL.
// e.g. "https://github.com" -> "github", "https://accounts.google.com" -> "google"
func (p *Provider) ProviderName() string {
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
func (p *Provider) ExchangeCode(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*UserInfo, error) {
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
func (p *Provider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return p.oauth2Config.AuthCodeURL(state, opts...)
}

// OAuth2Config returns the underlying OAuth2 config (for device flow).
func (p *Provider) OAuth2Config() *oauth2.Config {
	return &p.oauth2Config
}
