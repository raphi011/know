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
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/oidc"
)

const (
	deviceCodeExpiry = 15 * time.Minute
	pollInterval     = 5 // seconds
)

// OIDCHandler holds dependencies for OIDC auth routes.
type OIDCHandler struct {
	provider             *oidc.Provider
	db                   *db.Client
	selfSignup           bool
	stateSecret          []byte // HMAC key for signing state parameters
	redirectURL          string // configured OIDC redirect URL, used to derive base URL
	tokenMaxLifetimeDays int
}

// NewOIDCHandler creates a new OIDC handler.
// Generates a random 32-byte state secret on creation.
func NewOIDCHandler(provider *oidc.Provider, dbClient *db.Client, selfSignup bool, redirectURL string, tokenMaxLifetimeDays int) (*OIDCHandler, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate state secret: %w", err)
	}
	return &OIDCHandler{
		provider:             provider,
		db:                   dbClient,
		selfSignup:           selfSignup,
		stateSecret:          secret,
		redirectURL:          redirectURL,
		tokenMaxLifetimeDays: tokenMaxLifetimeDays,
	}, nil
}

// RegisterRoutes registers unauthenticated OIDC auth routes.
func (h *OIDCHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/device/start", h.handleDeviceStart)
	mux.HandleFunc("POST /auth/device/poll", h.handleDevicePoll)
	mux.HandleFunc("POST /auth/token", h.handleTokenExchange)
	mux.HandleFunc("GET /auth/login", h.handleOIDCLogin)
	mux.HandleFunc("GET /auth/callback", h.handleOIDCCallback)
}

// baseURL derives the scheme+host from the configured redirect URL.
// e.g. "https://know.example.com/auth/callback" -> "https://know.example.com"
func (h *OIDCHandler) baseURL() string {
	u, err := url.Parse(h.redirectURL)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// handleDeviceStart initiates the device authorization flow.
// POST /auth/device/start
// Response: {user_code, verification_uri, device_code, expires_in, interval}
func (h *OIDCHandler) handleDeviceStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	userCode, deviceCode, err := oidc.GenerateDeviceCode()
	if err != nil {
		logger.Error("generate device code", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate device code")
		return
	}

	expiresAt := time.Now().Add(deviceCodeExpiry)
	if err := h.db.CreateDeviceCode(ctx, deviceCode, userCode, expiresAt); err != nil {
		logger.Error("store device code", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to store device code")
		return
	}

	verificationURI := h.baseURL() + "/auth/login?user_code=" + userCode

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
// Responses:
//   - 200 {token} — approved, token returned
//   - 428 {error: "authorization_pending"} — not yet approved
//   - 410 {error: "expired"} — device code expired
//   - 404 — device code not found
func (h *OIDCHandler) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	body, ok := decodeBody[struct {
		DeviceCode string `json:"device_code"`
	}](w, r, 1024)
	if !ok {
		return
	}

	if body.DeviceCode == "" {
		writeError(w, http.StatusBadRequest, "device_code is required")
		return
	}

	dc, err := h.db.GetDeviceCodeByCode(ctx, body.DeviceCode)
	if err != nil {
		logger.Error("get device code", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up device code")
		return
	}
	if dc == nil {
		writeError(w, http.StatusNotFound, "device code not found")
		return
	}

	if time.Now().After(dc.ExpiresAt) {
		// Clean up expired code
		if err := h.db.DeleteDeviceCode(ctx, body.DeviceCode); err != nil {
			logger.Warn("delete expired device code", "error", err)
		}
		writeError(w, http.StatusGone, "expired")
		return
	}

	if !dc.Approved {
		writeError(w, http.StatusPreconditionRequired, "authorization_pending")
		return
	}

	// Approved — return the token and delete the device code record
	token := ""
	if dc.TokenHash != nil {
		token = *dc.TokenHash
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
func (h *OIDCHandler) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	userCode := r.URL.Query().Get("user_code")
	if userCode == "" {
		writeError(w, http.StatusBadRequest, "user_code query parameter is required")
		return
	}

	// Verify the user code exists and is not expired
	dc, err := h.db.GetDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		logger.Error("get device code by user code", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to look up device code")
		return
	}
	if dc == nil {
		writeError(w, http.StatusNotFound, "invalid or expired user code")
		return
	}
	if time.Now().After(dc.ExpiresAt) {
		writeError(w, http.StatusGone, "user code has expired")
		return
	}

	state := oidc.SignState(h.stateSecret, userCode)
	authURL := h.provider.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback handles the OIDC provider's redirect after authentication.
// GET /auth/callback?state=...&code=...
func (h *OIDCHandler) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	if state == "" || code == "" {
		writeError(w, http.StatusBadRequest, "missing state or code parameter")
		return
	}

	userCode, ok := oidc.VerifyState(h.stateSecret, state)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}

	// Exchange authorization code for user info
	userInfo, err := h.provider.ExchangeCode(ctx, code)
	if err != nil {
		logger.Error("exchange oidc code", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to exchange authorization code")
		return
	}

	// Resolve or create the user
	user, err := oidc.FindOrCreateUser(ctx, h.db, userInfo, h.selfSignup)
	if err != nil {
		logger.Error("find or create user", "error", err)
		writeError(w, http.StatusForbidden, "login failed: "+err.Error())
		return
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		logger.Error("extract user id", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to extract user ID")
		return
	}

	// Generate an API token for the CLI
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		logger.Error("generate token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	tokenExpiry := time.Now().Add(time.Duration(h.tokenMaxLifetimeDays) * 24 * time.Hour)
	if _, err := h.db.CreateToken(ctx, userID, tokenHash, "oidc-device-login", tokenExpiry); err != nil {
		logger.Error("create token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
		return
	}

	// Approve the device code with the raw token so the CLI can retrieve it
	if err := h.db.ApproveDeviceCode(ctx, userCode, userID, rawToken); err != nil {
		logger.Error("approve device code", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to approve device code")
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

// handleTokenExchange handles POST /auth/token
// This is for native apps using PKCE flow.
// Request body: {code, code_verifier, redirect_uri}
// Response: {token: "kh_...", user: {id, name, email}}
func (h *OIDCHandler) handleTokenExchange(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "code and code_verifier are required")
		return
	}

	// Exchange with PKCE verifier
	userInfo, err := h.provider.ExchangeCode(ctx, body.Code,
		oauth2.SetAuthURLParam("code_verifier", body.CodeVerifier),
	)
	if err != nil {
		logger.Error("exchange code with pkce", "error", err)
		writeError(w, http.StatusBadRequest, "failed to exchange authorization code")
		return
	}

	// Resolve or create user
	user, err := oidc.FindOrCreateUser(ctx, h.db, userInfo, h.selfSignup)
	if err != nil {
		logger.Error("find or create user", "error", err)
		writeError(w, http.StatusForbidden, "login failed: "+err.Error())
		return
	}

	userID, err := models.RecordIDString(user.ID)
	if err != nil {
		logger.Error("token exchange: extract user id", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to extract user ID")
		return
	}

	// Generate API token
	rawToken, tokenHash, err := auth.GenerateToken()
	if err != nil {
		logger.Error("generate token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	tokenExpiry := time.Now().Add(time.Duration(h.tokenMaxLifetimeDays) * 24 * time.Hour)
	if _, err := h.db.CreateToken(ctx, userID, tokenHash, "oidc-pkce-login", tokenExpiry); err != nil {
		logger.Error("create token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create token")
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
