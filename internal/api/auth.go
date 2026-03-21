package api

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/httputil"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/oidc"
)

const (
	deviceCodeExpiry = 15 * time.Minute
	pollInterval     = 5 // seconds
)

// AuthHandler holds dependencies for OAuth/OIDC auth routes.
type AuthHandler struct {
	provider             oidc.Provider
	db                   *db.Client
	selfSignup           bool
	stateSecret          []byte // HMAC key for signing state parameters; regenerated each startup, invalidating in-flight device flows
	baseURL              string // scheme+host derived from redirect URL (e.g. "https://know.example.com")
	tokenMaxLifetimeDays int
}

// NewAuthHandler creates a new auth handler.
// Generates a random 32-byte state secret on creation.
// Returns an error if the redirectURL is malformed.
func NewAuthHandler(provider oidc.Provider, dbClient *db.Client, selfSignup bool, redirectURL string, tokenMaxLifetimeDays int) (*AuthHandler, error) {
	u, err := url.Parse(redirectURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid redirect URL %q: must be an absolute URL with scheme and host", redirectURL)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate state secret: %w", err)
	}
	return &AuthHandler{
		provider:             provider,
		db:                   dbClient,
		selfSignup:           selfSignup,
		stateSecret:          secret,
		baseURL:              u.Scheme + "://" + u.Host,
		tokenMaxLifetimeDays: tokenMaxLifetimeDays,
	}, nil
}

// RegisterRoutes registers unauthenticated OIDC auth routes.
// If mw is non-nil, each route is wrapped with the middleware (e.g. rate limiting).
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux, mw ...func(http.Handler) http.Handler) {
	wrap := func(handler http.HandlerFunc) http.Handler {
		var h http.Handler = handler
		for _, m := range mw {
			h = m(h)
		}
		return h
	}
	mux.Handle("POST /auth/device/start", wrap(h.handleDeviceStart))
	mux.Handle("POST /auth/device/poll", wrap(h.handleDevicePoll))
	mux.Handle("POST /auth/token", wrap(h.handleTokenExchange))
	mux.Handle("GET /auth/login", wrap(h.handleOIDCLogin))
	mux.Handle("GET /auth/callback", wrap(h.handleOIDCCallback))
}

// handleDeviceStart initiates the device authorization flow.
// POST /auth/device/start
// Response: {user_code, verification_uri, device_code, expires_in, interval}
func (h *AuthHandler) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	userCode, deviceCode, err := oidc.GenerateDeviceCode()
	if err != nil {
		logger.Error("generate device code", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to generate device code")
		return
	}

	expiresAt := time.Now().Add(deviceCodeExpiry)
	if err := h.db.CreateDeviceCode(ctx, deviceCode, userCode, expiresAt); err != nil {
		logger.Error("store device code", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to store device code")
		return
	}

	verificationURI := h.baseURL + "/auth/login?user_code=" + userCode

	writeJSON(w, http.StatusOK, map[string]any{
		"user_code":        oidc.FormatUserCode(userCode),
		"verification_uri": verificationURI,
		"device_code":      deviceCode,
		"expires_in":       int(deviceCodeExpiry.Seconds()),
		"interval":         pollInterval,
	})
}

// handleDevicePoll checks whether a device code has been approved.
// POST /auth/device/poll
// Request body: {device_code}
// Responses use RFC 9457 Problem Details (application/problem+json):
//   - 200 {token} — approved, token returned
//   - 428 {detail: "authorization_pending"} — not yet approved
//   - 410 {detail: "expired"} — device code expired
//   - 404 — device code not found
func (h *AuthHandler) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	body, ok := decodeBody[struct {
		DeviceCode string `json:"device_code"`
	}](w, r, 1024)
	if !ok {
		return
	}

	if body.DeviceCode == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "device_code is required")
		return
	}

	dc, err := h.db.GetDeviceCodeByCode(ctx, body.DeviceCode)
	if err != nil {
		logger.Error("get device code", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to look up device code")
		return
	}
	if dc == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "device code not found")
		return
	}

	if time.Now().After(dc.ExpiresAt) {
		// Clean up expired code
		if err := h.db.DeleteDeviceCode(ctx, body.DeviceCode); err != nil {
			logger.Warn("delete expired device code", "error", err)
		}
		httputil.WriteProblem(w, http.StatusGone, "expired")
		return
	}

	if !dc.Approved {
		httputil.WriteProblem(w, http.StatusPreconditionRequired, "authorization_pending")
		return
	}

	// Approved — decrypt and return the token, then delete the device code record
	encryptedToken := ""
	if dc.RawToken != nil {
		encryptedToken = *dc.RawToken
	}
	if encryptedToken == "" {
		logger.Error("approved device code has no token", "device_code", body.DeviceCode)
		httputil.WriteProblem(w, http.StatusInternalServerError, "device code approved but token missing")
		return
	}

	token, err := oidc.DecryptWithSecret(encryptedToken, body.DeviceCode)
	if err != nil {
		logger.Error("decrypt device code token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to decrypt token")
		return
	}

	if err := h.db.DeleteDeviceCode(ctx, body.DeviceCode); err != nil {
		logger.Warn("delete approved device code", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
	})
}

// handleOIDCLogin redirects the user to the OIDC provider for authentication.
// GET /auth/login?user_code=ABCDEFGH
func (h *AuthHandler) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	userCode := r.URL.Query().Get("user_code")
	if userCode == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "user_code query parameter is required")
		return
	}

	// Verify the user code exists and is not expired
	dc, err := h.db.GetDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		logger.Error("get device code by user code", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to look up device code")
		return
	}
	if dc == nil {
		httputil.WriteProblem(w, http.StatusNotFound, "invalid or expired user code")
		return
	}
	if time.Now().After(dc.ExpiresAt) {
		httputil.WriteProblem(w, http.StatusGone, "user code has expired")
		return
	}

	state := oidc.SignState(h.stateSecret, userCode)
	authURL := h.provider.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback dispatches the OIDC provider's redirect to the appropriate flow.
// GET /auth/callback?state=...&code=...
func (h *AuthHandler) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	oauthError := r.URL.Query().Get("error")

	// Handle upstream provider errors (user denied consent, etc.)
	if oauthError != "" {
		if oidc.IsOAuthState(state) {
			payload, err := oidc.VerifyOAuthState(h.stateSecret, state)
			if err == nil {
				redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "access_denied", r.URL.Query().Get("error_description"))
				return
			}
			logutil.FromCtx(r.Context()).Warn("oauth state verification failed", "error", err)
		}
		httputil.WriteProblem(w, http.StatusBadRequest, "authentication failed: "+oauthError)
		return
	}

	if state == "" || code == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "missing state or code parameter")
		return
	}

	if oidc.IsOAuthState(state) {
		h.handleOAuthCallback(w, r, state, code)
		return
	}

	h.handleDeviceFlowCallback(w, r, state, code)
}

// handleDeviceFlowCallback completes the device authorization flow after OIDC authentication.
func (h *AuthHandler) handleDeviceFlowCallback(w http.ResponseWriter, r *http.Request, state, code string) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	userCode, ok := oidc.VerifyState(h.stateSecret, state)
	if !ok {
		httputil.WriteProblem(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	// Exchange authorization code for user info
	userInfo, err := h.provider.ExchangeCode(ctx, code)
	if err != nil {
		logger.Error("exchange oidc code", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to exchange authorization code")
		return
	}

	// Resolve or create the user
	user, err := oidc.FindOrCreateUser(ctx, h.db, userInfo, h.selfSignup)
	if err != nil {
		logger.Warn("find or create user failed", "error", err)
		httputil.WriteProblem(w, http.StatusForbidden, "login failed: user not found or registration disabled")
		return
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		logger.Error("extract user id", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to extract user ID")
		return
	}

	// Generate an API token for the CLI
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		logger.Error("generate token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	tokenExpiry := time.Now().Add(time.Duration(h.tokenMaxLifetimeDays) * 24 * time.Hour)
	token, err := h.db.CreateToken(ctx, userID, tokenHash, "oidc-device-login", tokenExpiry)
	if err != nil {
		logger.Error("create token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	// Encrypt the raw token with the device code before storing — only the
	// CLI that holds the device_code can decrypt it.
	dc, err := h.db.GetDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		logger.Error("get device code for encryption", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to look up device code")
		return
	}
	if dc == nil {
		logger.Warn("device code not found during callback", "user_code", userCode)
		httputil.WriteProblem(w, http.StatusNotFound, "device code not found or expired")
		return
	}
	encryptedToken, err := oidc.EncryptWithSecret(rawToken, dc.DeviceCode)
	if err != nil {
		logger.Error("encrypt token for device code", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to encrypt token")
		return
	}

	// Store the encrypted token so only the CLI can retrieve it.
	if err := h.db.ApproveDeviceCode(ctx, userCode, userID, encryptedToken); err != nil {
		logger.Error("approve device code", "error", err)
		// Clean up the orphaned token to avoid leaving a valid but undelivered credential
		tokenID, idErr := models.RecordIDString(token.ID)
		if idErr != nil {
			logger.Error("extract token id for cleanup", "error", idErr)
		} else if delErr := h.db.DeleteToken(ctx, tokenID); delErr != nil {
			logger.Error("delete orphaned token after device code approval failure", "error", delErr)
		}
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to approve device code")
		return
	}

	logger.Info("device flow completed", "user_id", userID, "provider", userInfo.Provider)

	// Return a simple HTML page telling the user to close the tab
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Login Successful</title></head>
<body style="font-family: system-ui, sans-serif; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; background: #f5f5f5;">
<div style="text-align: center; padding: 2rem; background: white; border-radius: 8px; box-shadow: 0 2px 8px rgba(0,0,0,0.1);">
<h1 style="color: #22c55e;">&#10003; Login Successful</h1>
<p>You can close this tab and return to your terminal.</p>
</div>
</body>
</html>`)
}

// handleOAuthCallback completes the OAuth authorization code flow after OIDC authentication.
func (h *AuthHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request, state, code string) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	payload, err := oidc.VerifyOAuthState(h.stateSecret, state)
	if err != nil {
		logger.Warn("invalid oauth state", "error", err)
		httputil.WriteProblem(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	// Exchange upstream authorization code for user info
	userInfo, err := h.provider.ExchangeCode(ctx, code)
	if err != nil {
		logger.Error("exchange oidc code", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "failed to exchange authorization code")
		return
	}

	// Resolve or create the user
	user, err := oidc.FindOrCreateUser(ctx, h.db, userInfo, h.selfSignup)
	if err != nil {
		logger.Warn("find or create user failed", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "access_denied", "login failed: user not found or registration disabled")
		return
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		logger.Error("extract user id", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "internal error")
		return
	}

	// Generate an API token for the MCP client
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		logger.Error("generate token", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "internal error")
		return
	}

	tokenExpiry := time.Now().Add(time.Duration(h.tokenMaxLifetimeDays) * 24 * time.Hour)
	token, err := h.db.CreateToken(ctx, userID, tokenHash, "oauth-mcp-login", tokenExpiry)
	if err != nil {
		logger.Error("create token", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "internal error")
		return
	}

	// Generate a short-lived auth code to pass back to the client
	authCode, err := generateAuthCode()
	if err != nil {
		logger.Error("generate auth code", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "internal error")
		return
	}

	// Encrypt the raw token with the code_challenge as key — only the client
	// that holds the code_verifier can derive the challenge and decrypt it.
	encryptedToken, err := oidc.EncryptWithSecret(rawToken, payload.CodeChallenge)
	if err != nil {
		logger.Error("encrypt token for oauth", "error", err)
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "internal error")
		return
	}

	if err := h.db.CreateOAuthAuthCode(ctx, authCode, encryptedToken, payload.CodeChallenge, payload.RedirectURI, time.Now().Add(oauthAuthCodeExpiry)); err != nil {
		logger.Error("create oauth auth code", "error", err)
		// Clean up the orphaned token to avoid leaving a valid but undelivered credential
		tokenID, idErr := models.RecordIDString(token.ID)
		if idErr != nil {
			logger.Error("extract token id for cleanup", "error", idErr)
		} else if delErr := h.db.DeleteToken(ctx, tokenID); delErr != nil {
			logger.Error("delete orphaned token after oauth auth code failure", "error", delErr)
		}
		redirectWithError(w, r, payload.RedirectURI, payload.ClientState, "server_error", "internal error")
		return
	}

	logger.Info("oauth flow completed", "user_id", userID, "provider", userInfo.Provider)

	// Redirect back to the client with the authorization code
	u, err := url.Parse(payload.RedirectURI)
	if err != nil {
		logger.Error("parse redirect uri", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "invalid redirect URI")
		return
	}
	q := u.Query()
	q.Set("code", authCode)
	if payload.ClientState != "" {
		q.Set("state", payload.ClientState)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// handleTokenExchange handles POST /auth/token
// This is for native apps using PKCE flow.
// Request body: {code, code_verifier, redirect_uri}
// Response: {token: "kh_...", user: {id, name, email}}
func (h *AuthHandler) handleTokenExchange(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	body, ok := decodeBody[struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
		RedirectURI  string `json:"redirect_uri"`
	}](w, r, 4096)
	if !ok {
		return
	}

	if body.Code == "" || body.CodeVerifier == "" {
		httputil.WriteProblem(w, http.StatusBadRequest, "code and code_verifier are required")
		return
	}

	// Exchange with PKCE verifier
	userInfo, err := h.provider.ExchangeCode(ctx, body.Code,
		oauth2.SetAuthURLParam("code_verifier", body.CodeVerifier),
	)
	if err != nil {
		logger.Error("exchange code with pkce", "error", err)
		httputil.WriteProblem(w, http.StatusBadRequest, "failed to exchange authorization code")
		return
	}

	// Resolve or create user
	user, err := oidc.FindOrCreateUser(ctx, h.db, userInfo, h.selfSignup)
	if err != nil {
		logger.Warn("find or create user failed", "error", err)
		httputil.WriteProblem(w, http.StatusForbidden, "login failed: user not found or registration disabled")
		return
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		logger.Error("token exchange: extract user id", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to extract user ID")
		return
	}

	// Generate API token
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		logger.Error("generate token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	tokenExpiry := time.Now().Add(time.Duration(h.tokenMaxLifetimeDays) * 24 * time.Hour)
	if _, err := h.db.CreateToken(ctx, userID, tokenHash, "oidc-pkce-login", tokenExpiry); err != nil {
		logger.Error("create token", "error", err)
		httputil.WriteProblem(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": rawToken,
		"user": map[string]any{
			"id":    userID,
			"name":  user.Name,
			"email": user.Email,
		},
	})
}
