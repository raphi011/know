package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/oidc"
)

const oauthAuthCodeExpiry = 60 * time.Second

type OAuthHandler struct {
	auth    *AuthHandler
	baseURL string
}

func NewOAuthHandler(authHandler *AuthHandler, baseURL string) *OAuthHandler {
	return &OAuthHandler{
		auth:    authHandler,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

func (h *OAuthHandler) RegisterRoutes(mux *http.ServeMux, mw ...func(http.Handler) http.Handler) {
	wrap := func(handler http.HandlerFunc) http.Handler {
		var hdlr http.Handler = handler
		for _, m := range mw {
			hdlr = m(hdlr)
		}
		return hdlr
	}
	mux.Handle("GET /.well-known/oauth-authorization-server", wrap(h.handleMetadata))
	mux.Handle("GET /oauth/authorize", wrap(h.handleAuthorize))
	mux.Handle("POST /oauth/token", wrap(h.handleToken))
}

func (h *OAuthHandler) handleMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                h.baseURL,
		"authorization_endpoint":                h.baseURL + "/oauth/authorize",
		"token_endpoint":                        h.baseURL + "/oauth/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

func (h *OAuthHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	redirectURI := q.Get("redirect_uri")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	state := q.Get("state")
	responseType := q.Get("response_type")

	if redirectURI == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "redirect_uri is required")
		return
	}
	if !isLoopbackRedirectURI(redirectURI) {
		httputil.WriteProblem(w, http.StatusBadRequest, "redirect_uri must be a loopback address (localhost, 127.0.0.1, or [::1])")
		return
	}

	if responseType != "code" {
		redirectWithError(w, r, redirectURI, state, "unsupported_response_type", "only response_type=code is supported")
		return
	}
	if codeChallenge == "" {
		redirectWithError(w, r, redirectURI, state, "invalid_request", "code_challenge is required")
		return
	}
	if codeChallengeMethod != "S256" {
		redirectWithError(w, r, redirectURI, state, "invalid_request", "code_challenge_method must be S256")
		return
	}

	oauthState, err := oidc.SignOAuthState(h.auth.stateSecret, oidc.OAuthStatePayload{
		RedirectURI:   redirectURI,
		CodeChallenge: codeChallenge,
		ClientState:   state,
	})
	if err != nil {
		logutil.FromCtx(r.Context()).Error("sign oauth state", "error", err)
		redirectWithError(w, r, redirectURI, state, "server_error", "internal error")
		return
	}

	authURL := h.auth.provider.AuthCodeURL(oauthState)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *OAuthHandler) handleToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "malformed request body")
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	redirectURI := r.FormValue("redirect_uri")

	if grantType != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code is supported")
		return
	}
	if code == "" || codeVerifier == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code and code_verifier are required")
		return
	}

	// Atomically consume the auth code to prevent replay attacks.
	ac, err := h.auth.db.ConsumeOAuthAuthCode(ctx, code)
	if err != nil {
		logger.Error("consume oauth auth code", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}
	if ac == nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "authorization code not found or expired")
		return
	}

	if redirectURI != "" && redirectURI != ac.RedirectURI {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri mismatch")
		return
	}

	hash := sha256.Sum256([]byte(codeVerifier))
	computedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	if computedChallenge != ac.CodeChallenge {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "PKCE verification failed")
		return
	}

	rawToken, err := oidc.DecryptWithSecret(ac.EncryptedToken, ac.CodeChallenge)
	if err != nil {
		logger.Error("decrypt oauth token", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to decrypt token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": rawToken,
		"token_type":   "bearer",
	})
}

func isLoopbackRedirectURI(rawURI string) bool {
	u, err := url.Parse(rawURI)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// redirectWithError redirects the client back to redirectURI with OAuth error parameters.
// Precondition: redirectURI must have been validated as a loopback URI before calling.
func redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, state, errorCode, description string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid redirect URI")
		return
	}
	q := u.Query()
	q.Set("error", errorCode)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func writeOAuthError(w http.ResponseWriter, status int, errorCode, description string) {
	writeJSON(w, status, map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func generateAuthCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate auth code: %w", err)
	}
	return hex.EncodeToString(b), nil
}
