// Package web provides the Templ + HTMX web frontend for knowhow.
package web

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/vault"
	"github.com/raphi011/knowhow/internal/web/templates/pages"
)

//go:embed static
var staticFS embed.FS

// Handler holds dependencies and registers web UI routes.
type Handler struct {
	db       *db.Client
	vaultSvc *vault.Service
	sessions *SessionStore
}

// NewHandler creates a new web UI handler.
func NewHandler(
	dbClient *db.Client,
	vaultSvc *vault.Service,
) *Handler {
	return &Handler{
		db:       dbClient,
		vaultSvc: vaultSvc,
		sessions: NewSessionStore(24 * time.Hour),
	}
}

// Register adds web UI routes to the mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Static assets (no auth)
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Embedded FS should never fail — indicates a build problem
		panic(fmt.Sprintf("failed to create static sub-filesystem: %v", err))
	}
	mux.Handle("/static/", cacheStatic(http.StripPrefix("/static/", http.FileServer(http.FS(staticSub)))))

	// Login (no auth, but CSRF-protected for POST)
	mux.HandleFunc("GET /login", h.handleLoginPage)
	mux.Handle("POST /login", CSRFMiddleware(http.HandlerFunc(h.handleLoginSubmit)))

	// Logout (no auth check needed, just clear cookie, CSRF-protected)
	mux.Handle("POST /logout", CSRFMiddleware(http.HandlerFunc(h.handleLogout)))

	// Session-protected routes
	sessionMw := SessionMiddleware(h.sessions)
	mux.Handle("GET /{$}", http.RedirectHandler("/agent", http.StatusSeeOther))
	mux.Handle("GET /agent", sessionMw(http.HandlerFunc(h.handleAgentPage)))

	// Settings page
	mux.Handle("GET /settings", sessionMw(http.HandlerFunc(h.handleSettingsPage)))

	// HTMX partials (session-protected)
	mux.Handle("POST /hx/agent/new", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleAgentNew))))
	mux.Handle("POST /hx/vault/switch", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleVaultSwitch))))
	mux.Handle("POST /hx/settings/locale", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleLocaleChange))))
	mux.Handle("POST /hx/settings/theme", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleThemeChange))))
}

func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to agent
	if sess := h.sessions.FromRequest(r); sess != nil {
		http.Redirect(w, r, "/agent", http.StatusSeeOther)
		return
	}

	locale := localeFromRequest(r)
	h.renderLogin(w, r, locale, "")
}

func (h *Handler) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	locale := localeFromRequest(r)
	token := r.FormValue("token")
	t := T(locale)

	if token == "" {
		h.renderLogin(w, r, locale, t("login.error.empty"))
		return
	}

	info, err := auth.ValidateToken(r.Context(), h.db, token)
	if err != nil {
		slog.Warn("web: login failed", "error", err)
		h.renderLogin(w, r, locale, t("login.error.invalid"))
		return
	}
	vaultPerms := info.Vaults

	// Pick default vault
	selectedVault := ""
	if len(vaultPerms) > 0 {
		selectedVault = vaultPerms[0].VaultID
	}

	sess := h.sessions.Create(info.UserID, vaultPerms, selectedVault)
	sess.Locale = locale
	h.sessions.SetCookie(w, sess)

	http.Redirect(w, r, "/agent", http.StatusSeeOther)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sess := h.sessions.FromRequest(r); sess != nil {
		h.sessions.Delete(sess.ID)
	}
	h.sessions.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, locale, errMsg string) {
	t := T(locale)
	component := pages.LoginPage(pages.LoginData{
		Error:  errMsg,
		Locale: locale,
		Theme:  themeFromRequest(r),
		T:      t,
	})
	renderPage(w, r, component)
}

func (h *Handler) handleAgentPage(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	t := T(sess.Locale)

	// List conversations
	convs, err := h.db.ListConversations(r.Context(), sess.SelectedVault, sess.UserID)
	if err != nil {
		slog.Error("web: failed to list conversations", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var agentConvs []pages.AgentConversation
	for _, c := range convs {
		id, err := models.RecordIDString(c.ID)
		if err != nil {
			slog.Warn("web: failed to extract conversation ID", "error", err)
			continue
		}
		agentConvs = append(agentConvs, pages.AgentConversation{
			ID:    id,
			Title: c.Title,
		})
	}

	// Load selected conversation messages if any
	convID := r.URL.Query().Get("conv")
	var agentMsgs []pages.AgentMessage

	if convID != "" {
		msgs, err := h.db.ListMessages(r.Context(), convID)
		if err != nil {
			slog.Error("web: failed to list messages", "conv", convID, "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, m := range msgs {
			if m.Role == models.RoleUser || m.Role == models.RoleAssistant {
				content := m.Content
				if m.Role == models.RoleAssistant {
					rendered, err := RenderMarkdown(content)
					if err != nil {
						slog.Warn("web: failed to render markdown for message", "conv", convID, "error", err)
					} else {
						content = rendered
					}
				}
				agentMsgs = append(agentMsgs, pages.AgentMessage{
					Role:    string(m.Role),
					Content: content,
				})
			}
		}
	}

	component := pages.AgentPage(pages.AgentPageData{
		Locale:         sess.Locale,
		Theme:          sess.Theme,
		VaultID:        sess.SelectedVault,
		ConversationID: convID,
		Conversations:  agentConvs,
		Messages:       agentMsgs,
		T:              t,
	})

	renderPage(w, r, component)
}

func (h *Handler) handleAgentNew(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("HX-Redirect", "/agent")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	t := T(sess.Locale)

	// List all vaults the user has access to
	var vaultOpts []pages.VaultOption
	for _, vp := range sess.VaultPermissions {
		v, err := h.vaultSvc.Get(r.Context(), vp.VaultID)
		if err != nil {
			slog.Warn("web: failed to fetch vault name", "vault", vp.VaultID, "error", err)
		}
		name := vp.VaultID
		if v != nil {
			name = v.Name
		}
		vaultOpts = append(vaultOpts, pages.VaultOption{
			ID:       vp.VaultID,
			Name:     name,
			Selected: vp.VaultID == sess.SelectedVault,
		})
	}

	component := pages.SettingsPage(pages.SettingsData{
		Locale: sess.Locale,
		Theme:  sess.Theme,
		Vaults: vaultOpts,
		T:      t,
	})

	renderPage(w, r, component)
}

func (h *Handler) handleVaultSwitch(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	vaultID := r.FormValue("vault")

	if vaultID == "" {
		http.Error(w, "vault parameter required", http.StatusBadRequest)
		return
	}

	if !hasVaultAccess(sess, vaultID) {
		http.Error(w, "unauthorized vault", http.StatusForbidden)
		return
	}

	sess.SetSelectedVault(vaultID)
	w.Header().Set("HX-Redirect", "/agent")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleLocaleChange(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	locale := r.FormValue("locale")

	if locale != "en" && locale != "de" {
		http.Error(w, "invalid locale", http.StatusBadRequest)
		return
	}

	sess.SetLocale(locale)

	// Also set cookie for pre-login fallback
	http.SetCookie(w, &http.Cookie{
		Name:     "kh_locale",
		Value:    locale,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("HX-Redirect", "/settings")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleThemeChange(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	theme := r.FormValue("theme")

	if theme != "light" && theme != "dark" && theme != "system" {
		http.Error(w, "invalid theme", http.StatusBadRequest)
		return
	}

	sess.SetTheme(theme)

	http.SetCookie(w, &http.Cookie{
		Name:     "kh_theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: false, // JS needs to read this for FOUC prevention
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("HX-Redirect", "/settings")
	w.WriteHeader(http.StatusOK)
}

// renderComponent is a minimal interface matching templ's Component.Render method.
type renderComponent interface {
	Render(ctx context.Context, w io.Writer) error
}

// renderPage renders a templ component to a buffer first, then writes to the response.
// This prevents partial HTML writes on render errors.
func renderPage(w http.ResponseWriter, r *http.Request, component renderComponent) {
	var buf bytes.Buffer
	if err := component.Render(r.Context(), &buf); err != nil {
		slog.Error("web: failed to render page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// cacheStatic wraps a handler with Cache-Control headers for static assets.
func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
		h.ServeHTTP(w, r)
	})
}

// hasVaultAccess checks whether the session includes the given vault ID.
func hasVaultAccess(sess *Session, vaultID string) bool {
	for _, vp := range sess.VaultPermissions {
		if vp.VaultID == vaultID || vp.VaultID == "*" {
			return true
		}
	}
	return false
}

// localeFromRequest reads locale from cookie, defaults to "en".
func localeFromRequest(r *http.Request) string {
	if c, err := r.Cookie("kh_locale"); err == nil && (c.Value == "en" || c.Value == "de") {
		return c.Value
	}
	return "en"
}

// themeFromRequest reads theme from cookie, defaults to "system".
func themeFromRequest(r *http.Request) string {
	if c, err := r.Cookie("kh_theme"); err == nil {
		if c.Value == "light" || c.Value == "dark" || c.Value == "system" {
			return c.Value
		}
	}
	return "system"
}
