package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/oidc"
)

const oauthAuthCodeExpiry = 60 * time.Second

type OAuthHandler struct {
	auth    *AuthHandler
	db      *db.Client
	baseURL string
}

func NewOAuthHandler(authHandler *AuthHandler, dbClient *db.Client, baseURL string) *OAuthHandler {
	return &OAuthHandler{
		auth:    authHandler,
		db:      dbClient,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// ResourceMetadataURL returns the URL of the Protected Resource Metadata document (RFC 9728).
func (h *OAuthHandler) ResourceMetadataURL() string {
	return h.baseURL + "/.well-known/oauth-protected-resource"
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
	mux.Handle("GET /.well-known/oauth-protected-resource", wrap(h.handleResourceMetadata))
	mux.Handle("GET /oauth/authorize", wrap(h.handleAuthorize))
	mux.Handle("POST /oauth/token", wrap(h.handleToken))
	mux.Handle("POST /oauth/register", wrap(h.handleRegister))
}

func (h *OAuthHandler) handleMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                h.baseURL,
		"authorization_endpoint":                h.baseURL + "/oauth/authorize",
		"token_endpoint":                        h.baseURL + "/oauth/token",
		"registration_endpoint":                 h.baseURL + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

// handleResourceMetadata serves OAuth 2.0 Protected Resource Metadata (RFC 9728).
// MCP clients use this to discover the authorization server for this resource.
func (h *OAuthHandler) handleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":              h.baseURL + "/mcp",
		"authorization_servers": []string{h.baseURL},
	})
}

func (h *OAuthHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)
	q := r.URL.Query()

	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	state := q.Get("state")
	responseType := q.Get("response_type")

	if clientID == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "client_id is required")
		return
	}

	client, err := h.db.GetOAuthClient(ctx, clientID)
	if err != nil {
		logger.Error("look up oauth client", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "internal error")
		return
	}
	if client == nil {
		httputil.WriteProblem(w, http.StatusBadRequest, "unknown client_id")
		return
	}

	if redirectURI == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "redirect_uri is required")
		return
	}
	if !isRegisteredRedirectURI(redirectURI, client.RedirectURIs) {
		httputil.WriteProblem(w, http.StatusBadRequest, "redirect_uri does not match any registered redirect URI for this client")
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

// handleRegister implements RFC 7591 Dynamic Client Registration.
// Accepts client metadata and returns a client_id for use in the OAuth flow.
func (h *OAuthHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64 KB limit

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Debug("decode register request", "error", err)
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "malformed request body")
		return
	}

	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "redirect_uris is required")
		return
	}
	for _, uri := range req.RedirectURIs {
		if !isLoopbackRedirectURI(uri) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri",
				fmt.Sprintf("redirect_uri %q must be a loopback address (localhost, 127.0.0.1, or [::1])", uri))
			return
		}
	}

	clientID := uuid.New().String()

	if err := h.db.CreateOAuthClient(ctx, clientID, req.ClientName, req.RedirectURIs); err != nil {
		logger.Error("create oauth client", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}

	logger.Info("oauth client registered", "client_id", clientID, "client_name", req.ClientName)

	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  clientID,
		"client_name":                req.ClientName,
		"redirect_uris":              req.RedirectURIs,
		"grant_types":                []string{"authorization_code"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
	})
}

// isRegisteredRedirectURI checks if the redirect URI matches one of the client's
// registered URIs. Per RFC 7591, the port is ignored for loopback URIs since
// native clients bind to ephemeral ports.
func isRegisteredRedirectURI(rawURI string, registered []string) bool {
	u, err := url.Parse(rawURI)
	if err != nil {
		return false
	}
	for _, reg := range registered {
		ru, err := url.Parse(reg)
		if err != nil {
			continue
		}
		// Compare scheme, host (without port), and path.
		if u.Scheme == ru.Scheme && u.Hostname() == ru.Hostname() && u.Path == ru.Path {
			return true
		}
	}
	return false
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
