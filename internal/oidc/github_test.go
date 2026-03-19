package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestGitHubProviderName(t *testing.T) {
	p := NewGitHub("id", "secret", "http://localhost/callback", "")
	if got := p.ProviderName(); got != "github" {
		t.Errorf("ProviderName() = %q, want %q", got, "github")
	}

	p2 := NewGitHub("id", "secret", "http://localhost/callback", "my-github")
	if got := p2.ProviderName(); got != "my-github" {
		t.Errorf("ProviderName() = %q, want %q", got, "my-github")
	}
}

func TestFetchUser(t *testing.T) {
	tests := []struct {
		name       string
		response   any
		statusCode int
		wantErr    bool
		wantID     int64
		wantLogin  string
		wantName   string
	}{
		{
			name:       "normal user",
			response:   githubUser{ID: 12345, Login: "octocat", Name: "The Octocat", Email: "octocat@github.com"},
			statusCode: http.StatusOK,
			wantID:     12345,
			wantLogin:  "octocat",
			wantName:   "The Octocat",
		},
		{
			name:       "user without name",
			response:   githubUser{ID: 99, Login: "ghost"},
			statusCode: http.StatusOK,
			wantID:     99,
			wantLogin:  "ghost",
		},
		{
			name:       "user with zero id",
			response:   githubUser{Login: "noone"},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		{
			name:       "server error",
			response:   map[string]string{"message": "internal"},
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			g := &GitHubProvider{apiURL: srv.URL}
			user, err := g.fetchUser(srv.Client())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", user.ID, tt.wantID)
			}
			if user.Login != tt.wantLogin {
				t.Errorf("Login = %q, want %q", user.Login, tt.wantLogin)
			}
			if user.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", user.Name, tt.wantName)
			}
		})
	}
}

func TestFetchPrimaryEmail(t *testing.T) {
	tests := []struct {
		name       string
		emails     []githubEmail
		statusCode int
		wantEmail  string
		wantErr    bool
	}{
		{
			name: "primary verified email",
			emails: []githubEmail{
				{Email: "secondary@example.com", Primary: false, Verified: true},
				{Email: "primary@example.com", Primary: true, Verified: true},
			},
			statusCode: http.StatusOK,
			wantEmail:  "primary@example.com",
		},
		{
			name: "no primary, falls back to verified",
			emails: []githubEmail{
				{Email: "unverified@example.com", Primary: true, Verified: false},
				{Email: "verified@example.com", Primary: false, Verified: true},
			},
			statusCode: http.StatusOK,
			wantEmail:  "verified@example.com",
		},
		{
			name: "no verified emails returns error",
			emails: []githubEmail{
				{Email: "first@example.com", Primary: false, Verified: false},
			},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		{
			name:       "empty list",
			emails:     []githubEmail{},
			statusCode: http.StatusOK,
			wantErr:    true,
		},
		{
			name:       "api error",
			statusCode: http.StatusForbidden,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.emails != nil {
					json.NewEncoder(w).Encode(tt.emails)
				}
			}))
			defer srv.Close()

			g := &GitHubProvider{apiURL: srv.URL}
			email, err := g.fetchPrimaryEmail(srv.Client())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if email != tt.wantEmail {
				t.Errorf("email = %q, want %q", email, tt.wantEmail)
			}
		})
	}
}

func TestExchangeCode(t *testing.T) {
	tests := []struct {
		name        string
		user        githubUser
		emails      []githubEmail
		wantSubject string
		wantEmail   string
		wantName    string
		wantErr     bool
	}{
		{
			name:        "user with public email and name",
			user:        githubUser{ID: 12345, Login: "octocat", Name: "The Octocat", Email: "octocat@github.com"},
			wantSubject: "12345",
			wantEmail:   "octocat@github.com",
			wantName:    "The Octocat",
		},
		{
			name:        "email fallback to primary verified",
			user:        githubUser{ID: 42, Login: "ghost", Name: "Ghost"},
			emails:      []githubEmail{{Email: "ghost@example.com", Primary: true, Verified: true}},
			wantSubject: "42",
			wantEmail:   "ghost@example.com",
			wantName:    "Ghost",
		},
		{
			name:        "name fallback to login",
			user:        githubUser{ID: 99, Login: "bot"},
			emails:      []githubEmail{{Email: "bot@example.com", Primary: true, Verified: true}},
			wantSubject: "99",
			wantEmail:   "bot@example.com",
			wantName:    "bot",
		},
		{
			name:        "email fetch fails gracefully",
			user:        githubUser{ID: 77, Login: "private"},
			emails:      nil, // will cause /user/emails to 404
			wantSubject: "77",
			wantEmail:   "",
			wantName:    "private",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock GitHub API endpoints.
			apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/user":
					json.NewEncoder(w).Encode(tt.user)
				case "/user/emails":
					if tt.emails == nil {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					json.NewEncoder(w).Encode(tt.emails)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer apiSrv.Close()

			// Mock OAuth2 token endpoint.
			tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"access_token": "test-token",
					"token_type":   "bearer",
				})
			}))
			defer tokenSrv.Close()

			g := &GitHubProvider{
				oauth2Config: oauth2.Config{
					ClientID:     "test-id",
					ClientSecret: "test-secret",
					Endpoint: oauth2.Endpoint{
						TokenURL: tokenSrv.URL,
					},
				},
				providerName: "github",
				apiURL:       apiSrv.URL,
			}

			info, err := g.ExchangeCode(context.Background(), "test-code")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if info.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", info.Subject, tt.wantSubject)
			}
			if info.Email != tt.wantEmail {
				t.Errorf("Email = %q, want %q", info.Email, tt.wantEmail)
			}
			if info.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", info.Name, tt.wantName)
			}
			if info.Provider != "github" {
				t.Errorf("Provider = %q, want %q", info.Provider, "github")
			}
		})
	}
}
