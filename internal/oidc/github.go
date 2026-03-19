package oidc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"

	"github.com/raphi011/know/internal/logutil"
)

const gitHubAPIURL = "https://api.github.com"

// GitHubProvider implements Provider using GitHub's OAuth2 API.
// GitHub does not support OIDC discovery or id_tokens, so user info
// is fetched via the GitHub REST API (/user, /user/emails).
type GitHubProvider struct {
	oauth2Config oauth2.Config
	providerName string
	apiURL       string // overridable for testing
}

// NewGitHub creates a GitHub OAuth provider.
func NewGitHub(clientID, clientSecret, redirectURL string, providerName string) *GitHubProvider {
	if providerName == "" {
		providerName = "github"
	}
	return &GitHubProvider{
		oauth2Config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     github.Endpoint,
			RedirectURL:  redirectURL,
			Scopes:       []string{"read:user", "user:email"},
		},
		providerName: providerName,
		apiURL:       gitHubAPIURL,
	}
}

func (g *GitHubProvider) ProviderName() string {
	return g.providerName
}

func (g *GitHubProvider) AuthCodeURL(state string, opts ...oauth2.AuthCodeOption) string {
	return g.oauth2Config.AuthCodeURL(state, opts...)
}

func (g *GitHubProvider) ExchangeCode(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*UserInfo, error) {
	logger := logutil.FromCtx(ctx)

	token, err := g.oauth2Config.Exchange(ctx, code, opts...)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(token))

	user, err := g.fetchUser(client)
	if err != nil {
		return nil, fmt.Errorf("get user info: %w", err)
	}

	email := user.Email
	if email == "" {
		var emailErr error
		email, emailErr = g.fetchPrimaryEmail(client)
		if emailErr != nil {
			logger.Warn("could not fetch github email, proceeding without",
				"error", emailErr, "github_user_id", user.ID, "github_login", user.Login)
		}
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}

	return &UserInfo{
		Subject:  strconv.FormatInt(user.ID, 10),
		Email:    email,
		Name:     name,
		Provider: g.ProviderName(),
	}, nil
}

type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (g *GitHubProvider) fetchUser(client *http.Client) (*githubUser, error) {
	resp, err := client.Get(g.apiURL + "/user")
	if err != nil {
		return nil, fmt.Errorf("get /user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("get /user: status %d: %s", resp.StatusCode, body)
	}

	var user githubUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode /user: %w", err)
	}
	if user.ID == 0 {
		return nil, fmt.Errorf("github user has no id")
	}
	return &user, nil
}

func (g *GitHubProvider) fetchPrimaryEmail(client *http.Client) (string, error) {
	resp, err := client.Get(g.apiURL + "/user/emails")
	if err != nil {
		return "", fmt.Errorf("get /user/emails: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("get /user/emails: status %d: %s", resp.StatusCode, body)
	}

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decode /user/emails: %w", err)
	}

	// Prefer primary+verified, then any verified, then first verified-unchecked.
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	if len(emails) > 0 {
		return "", fmt.Errorf("no verified email addresses found (%d unverified)", len(emails))
	}
	return "", fmt.Errorf("no email addresses found")
}
