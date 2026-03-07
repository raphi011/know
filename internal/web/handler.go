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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/raphi011/knowhow/internal/auth"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/document"
	"github.com/raphi011/knowhow/internal/event"
	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/search"
	"github.com/raphi011/knowhow/internal/vault"
	"github.com/raphi011/knowhow/internal/web/templates/components"
	"github.com/raphi011/knowhow/internal/web/templates/pages"
	"github.com/raphi011/knowhow/internal/web/templates/partials"
)

//go:embed static
var staticFS embed.FS

// Handler holds dependencies and registers web UI routes.
type Handler struct {
	db         *db.Client
	docService *document.Service
	vaultSvc   *vault.Service
	searchSvc  *search.Service
	bus        *event.Bus
	sessions   *SessionStore
}

// NewHandler creates a new web UI handler.
func NewHandler(
	dbClient *db.Client,
	docService *document.Service,
	vaultSvc *vault.Service,
	searchSvc *search.Service,
	bus *event.Bus,
) *Handler {
	return &Handler{
		db:         dbClient,
		docService: docService,
		vaultSvc:   vaultSvc,
		searchSvc:  searchSvc,
		bus:        bus,
		sessions:   NewSessionStore(24 * time.Hour),
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
	mux.Handle("GET /{$}", sessionMw(http.HandlerFunc(h.handleIndex)))
	mux.Handle("GET /docs/{path...}", sessionMw(http.HandlerFunc(h.handleDocView)))

	// Agent page
	mux.Handle("GET /agent", sessionMw(http.HandlerFunc(h.handleAgentPage)))

	// Settings page
	mux.Handle("GET /settings", sessionMw(http.HandlerFunc(h.handleSettingsPage)))

	// HTMX partials (session-protected)
	mux.Handle("GET /hx/sidebar", sessionMw(http.HandlerFunc(h.handleSidebar)))
	mux.Handle("GET /hx/search", sessionMw(http.HandlerFunc(h.handleSearch)))
	mux.Handle("GET /hx/doc/events", sessionMw(http.HandlerFunc(h.handleDocEvents)))
	mux.Handle("GET /hx/versions", sessionMw(http.HandlerFunc(h.handleVersionList)))
	mux.Handle("GET /hx/version/diff", sessionMw(http.HandlerFunc(h.handleVersionDiff)))
	mux.Handle("POST /hx/vault/switch", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleVaultSwitch))))
	mux.Handle("POST /hx/settings/locale", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleLocaleChange))))
	mux.Handle("POST /hx/settings/theme", CSRFMiddleware(sessionMw(http.HandlerFunc(h.handleThemeChange))))
}

func (h *Handler) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	// If already logged in, redirect to index
	if sess := h.sessions.FromRequest(r); sess != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
	vaultAccess := info.VaultAccess

	// Pick default vault
	selectedVault := ""
	if len(vaultAccess) > 0 {
		selectedVault = vaultAccess[0]
	}

	sess := h.sessions.Create("web-user", vaultAccess, selectedVault)
	sess.Locale = locale
	h.sessions.SetCookie(w, sess)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if sess := h.sessions.FromRequest(r); sess != nil {
		h.sessions.Delete(sess.ID)
	}
	h.sessions.ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	t := T(sess.Locale)

	sidebar, err := h.buildSidebar(r.Context(), sess.SelectedVault)
	if err != nil {
		slog.Error("web: failed to build sidebar", "error", err)
	}

	// List documents in the selected vault
	docs, err := h.db.ListDocuments(r.Context(), db.ListDocumentsFilter{
		VaultID: sess.SelectedVault,
		Limit:   100,
	})
	if err != nil {
		slog.Error("web: failed to list documents", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var items []partials.DocListItem
	for _, doc := range docs {
		items = append(items, partials.DocListItem{
			Title:  doc.Title,
			Path:   doc.Path,
			Labels: doc.Labels,
		})
	}

	component := pages.DocsIndexPage(pages.DocsIndexData{
		Locale:        sess.Locale,
		Theme:         sess.Theme,
		SelectedVault: sess.SelectedVault,
		Sidebar:       sidebar,
		Documents:     items,
		T:             t,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render docs index", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleDocView(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	t := T(sess.Locale)

	// Decode path segments (catch-all route params are URL-encoded)
	rawPath := r.PathValue("path")
	segments := strings.Split(rawPath, "/")
	for i, s := range segments {
		decoded, err := decodePathSegment(s)
		if err == nil {
			segments[i] = decoded
		}
	}
	docPath := "/" + strings.Join(segments, "/")

	doc, err := h.db.GetDocumentByPath(r.Context(), sess.SelectedVault, docPath)
	if err != nil {
		slog.Error("web: failed to get document", "path", docPath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if doc == nil {
		http.NotFound(w, r)
		return
	}

	renderedHTML, err := RenderMarkdown(doc.ContentBody)
	if err != nil {
		slog.Error("web: failed to render markdown", "path", docPath, "error", err)
		renderedHTML = "<p>" + t("error.markdown_render_failed") + "</p>"
	}

	sidebar, err := h.buildSidebar(r.Context(), sess.SelectedVault)
	if err != nil {
		slog.Error("web: failed to build sidebar", "error", err)
	}

	docID, err := models.RecordIDString(doc.ID)
	if err != nil {
		slog.Error("web: failed to extract document ID", "path", docPath, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	component := pages.DocViewPage(pages.DocViewData{
		Locale:       sess.Locale,
		Theme:        sess.Theme,
		Title:        doc.Title,
		Path:         doc.Path,
		DocID:        docID,
		RenderedHTML: renderedHTML,
		Labels:       doc.Labels,
		VaultID:      sess.SelectedVault,
		Sidebar:      sidebar,
		T:            t,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render doc view", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleSidebar(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		vaultID = sess.SelectedVault
	}

	if !hasVaultAccess(sess, vaultID) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	sidebar, err := h.buildSidebar(r.Context(), vaultID)
	if err != nil {
		slog.Error("web: failed to build sidebar", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	renderBuffered(w, r, components.FolderTree(vaultID, sidebar.Folders))
}

// handleDocEvents streams SSE events for a specific document path.
func (h *Handler) handleDocEvents(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	vaultID := r.URL.Query().Get("vault")
	docPath := r.URL.Query().Get("path")

	if vaultID == "" || docPath == "" {
		http.Error(w, "vault and path required", http.StatusBadRequest)
		return
	}

	// Verify user has access to the requested vault
	if !hasVaultAccess(sess, vaultID) {
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	if h.bus == nil {
		http.Error(w, "events not available", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := h.bus.SubscribeByPath(vaultID, docPath)
	defer unsub()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			// Re-render the document and send as SSE event
			doc, err := h.db.GetDocumentByPath(r.Context(), vaultID, docPath)
			if err != nil || doc == nil {
				slog.Warn("web: doc events - failed to fetch doc for re-render",
					"path", docPath, "event", evt.Type, "error", err)
				continue
			}
			html, err := RenderMarkdown(doc.ContentBody)
			if err != nil {
				slog.Warn("web: doc events - failed to render markdown", "error", err)
				continue
			}
			if err := writeSSE(w, "doc-updated", html); err != nil {
				return // client disconnected
			}
			flusher.Flush()

		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return // client disconnected
			}
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}

func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, locale, errMsg string) {
	t := T(locale)
	component := pages.LoginPage(pages.LoginData{
		Error:  errMsg,
		Locale: locale,
		Theme:  themeFromRequest(r),
		T:      t,
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render login page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

// decodePathSegment URL-decodes a path segment.
func decodePathSegment(s string) (string, error) {
	return url.PathUnescape(s)
}

// renderComponent is a minimal interface matching templ's Component.Render method.
type renderComponent interface {
	Render(ctx context.Context, w io.Writer) error
}

// renderBuffered renders a templ component to a buffer first, then writes to the response.
// This prevents partial HTML writes on render errors (which would corrupt HTMX responses).
func renderBuffered(w http.ResponseWriter, r *http.Request, component renderComponent) {
	var buf bytes.Buffer
	if err := component.Render(r.Context(), &buf); err != nil {
		slog.Error("web: failed to render component", "error", err)
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
	for _, v := range sess.VaultAccess {
		if v == vaultID {
			return true
		}
	}
	return false
}

// writeSSE writes a properly formatted SSE event. Multi-line data gets each
// line prefixed with "data: " per the SSE spec. Returns an error if writing fails
// (e.g., client disconnected).
func writeSSE(w http.ResponseWriter, eventName, data string) error {
	if _, err := fmt.Fprintf(w, "event: %s\n", eventName); err != nil {
		return err
	}
	for _, line := range strings.Split(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(w, "\n")
	return err
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	q := r.URL.Query().Get("q")

	if q == "" {
		renderBuffered(w, r, partials.SearchResults(nil))
		return
	}

	results, err := h.searchSvc.Search(r.Context(), search.SearchInput{
		VaultID: sess.SelectedVault,
		Query:   q,
		Limit:   10,
	})
	if err != nil {
		slog.Error("web: search failed", "query", q, "error", err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}

	var items []partials.SearchResultItem
	for _, r := range results {
		snippet := ""
		if len(r.MatchedChunks) > 0 {
			snippet = r.MatchedChunks[0].Snippet
		}
		items = append(items, partials.SearchResultItem{
			Title:   r.Title,
			Path:    r.Path,
			Snippet: snippet,
			Labels:  r.Labels,
		})
	}

	renderBuffered(w, r, partials.SearchResults(items))
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
		}
		for _, m := range msgs {
			if m.Role == models.RoleUser || m.Role == models.RoleAssistant {
				content := m.Content
				if m.Role == models.RoleAssistant {
					rendered, err := RenderMarkdown(content)
					if err == nil {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render agent page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	t := T(sess.Locale)

	// List all vaults the user has access to
	var vaultOpts []pages.VaultOption
	for _, vid := range sess.VaultAccess {
		v, err := h.vaultSvc.Get(r.Context(), vid)
		if err != nil {
			slog.Warn("web: failed to fetch vault name", "vault", vid, "error", err)
		}
		name := vid
		if v != nil {
			name = v.Name
		}
		vaultOpts = append(vaultOpts, pages.VaultOption{
			ID:       vid,
			Name:     name,
			Selected: vid == sess.SelectedVault,
		})
	}

	component := pages.SettingsPage(pages.SettingsData{
		Locale: sess.Locale,
		Theme:  sess.Theme,
		Vaults: vaultOpts,
		T:      t,
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render settings page", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleVersionList(w http.ResponseWriter, r *http.Request) {
	docID := r.URL.Query().Get("doc")
	if docID == "" {
		http.Error(w, "doc parameter required", http.StatusBadRequest)
		return
	}

	versions, err := h.db.ListVersions(r.Context(), docID, 20, 0)
	if err != nil {
		slog.Error("web: failed to list versions", "doc", docID, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Determine current version number (latest)
	currentVersion := 0
	if len(versions) > 0 {
		currentVersion = versions[0].Version
	}

	var items []partials.VersionItem
	for _, v := range versions {
		items = append(items, partials.VersionItem{
			Version:   v.Version,
			Title:     v.Title,
			CreatedAt: v.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	renderBuffered(w, r, partials.VersionList(docID, items, currentVersion))
}

func (h *Handler) handleVersionDiff(w http.ResponseWriter, r *http.Request) {
	docID := r.URL.Query().Get("doc")
	versionStr := r.URL.Query().Get("v")
	currentStr := r.URL.Query().Get("current")

	if docID == "" || versionStr == "" || currentStr == "" {
		http.Error(w, "doc, v, and current parameters required", http.StatusBadRequest)
		return
	}

	oldVersion, err := strconv.Atoi(versionStr)
	if err != nil {
		http.Error(w, "invalid version number", http.StatusBadRequest)
		return
	}
	newVersion, err := strconv.Atoi(currentStr)
	if err != nil {
		http.Error(w, "invalid current version number", http.StatusBadRequest)
		return
	}

	oldVer, err := h.db.GetVersionByNumber(r.Context(), docID, oldVersion)
	if err != nil || oldVer == nil {
		slog.Error("web: failed to get old version", "doc", docID, "version", oldVersion, "error", err)
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}

	newVer, err := h.db.GetVersionByNumber(r.Context(), docID, newVersion)
	if err != nil || newVer == nil {
		slog.Error("web: failed to get new version", "doc", docID, "version", newVersion, "error", err)
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}

	lines := computeDiff(oldVer.Content, newVer.Content)

	renderBuffered(w, r, partials.DiffView(oldVersion, newVersion, lines))
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
	w.Header().Set("HX-Redirect", "/")
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
