// Package web provides the Templ + HTMX web frontend for knowhow.
package web

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
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
	noAuth     bool
}

// NewHandler creates a new web UI handler.
func NewHandler(
	dbClient *db.Client,
	docService *document.Service,
	vaultSvc *vault.Service,
	searchSvc *search.Service,
	bus *event.Bus,
	noAuth bool,
) *Handler {
	return &Handler{
		db:         dbClient,
		docService: docService,
		vaultSvc:   vaultSvc,
		searchSvc:  searchSvc,
		bus:        bus,
		sessions:   NewSessionStore(24 * time.Hour),
		noAuth:     noAuth,
	}
}

// Register adds web UI routes to the mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// Static assets (no auth)
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		slog.Error("failed to create static sub-filesystem", "error", err)
		return
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Login (no auth)
	mux.HandleFunc("GET /login", h.handleLoginPage)
	mux.HandleFunc("POST /login", h.handleLoginSubmit)

	// Logout (no auth check needed, just clear cookie)
	mux.HandleFunc("POST /logout", h.handleLogout)

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
	mux.Handle("POST /hx/vault/switch", sessionMw(http.HandlerFunc(h.handleVaultSwitch)))
	mux.Handle("POST /hx/settings/locale", sessionMw(http.HandlerFunc(h.handleLocaleChange)))
	mux.Handle("POST /hx/settings/theme", sessionMw(http.HandlerFunc(h.handleThemeChange)))
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

	hash := auth.HashToken(token)
	dbToken, err := h.db.GetTokenByHash(r.Context(), hash)
	if err != nil || dbToken == nil {
		slog.Warn("web: login failed", "error", err)
		h.renderLogin(w, r, locale, t("login.error.invalid"))
		return
	}

	// Extract vault access
	vaultAccess := make([]string, 0, len(dbToken.VaultAccess))
	for _, v := range dbToken.VaultAccess {
		id, err := models.RecordIDString(v)
		if err != nil {
			slog.Warn("web: failed to extract vault access ID", "error", err)
			continue
		}
		vaultAccess = append(vaultAccess, id)
	}

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
		renderedHTML = "<p>Failed to render markdown.</p>"
	}

	sidebar, err := h.buildSidebar(r.Context(), sess.SelectedVault)
	if err != nil {
		slog.Error("web: failed to build sidebar", "error", err)
	}

	docID, _ := models.RecordIDString(doc.ID)

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

	sidebar, err := h.buildSidebar(r.Context(), vaultID)
	if err != nil {
		slog.Error("web: failed to build sidebar", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := components.FolderTree(vaultID, sidebar.Folders)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render sidebar", "error", err)
	}
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

	_ = sess // used for auth context validation

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
			writeSSE(w, "doc-updated", html)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
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

// buildSidebar constructs the sidebar data by listing all folders in a vault
// and building them into a tree structure.
func (h *Handler) buildSidebar(ctx context.Context, vaultID string) (components.SidebarData, error) {
	v, err := h.vaultSvc.Get(ctx, vaultID)
	if err != nil {
		return components.SidebarData{VaultID: vaultID}, fmt.Errorf("get vault: %w", err)
	}
	vaultName := vaultID
	if v != nil {
		vaultName = v.Name
	}

	folders, err := h.vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		return components.SidebarData{VaultID: vaultID, VaultName: vaultName}, fmt.Errorf("list folders: %w", err)
	}

	tree := buildFolderTree(folders)

	return components.SidebarData{
		VaultID:   vaultID,
		VaultName: vaultName,
		Folders:   tree,
	}, nil
}

// buildFolderTree constructs a nested tree from a flat list of folders.
func buildFolderTree(folders []models.Folder) []components.FolderNode {
	// Group by parent path
	childrenOf := make(map[string][]components.FolderNode)
	for _, f := range folders {
		parent := path.Dir(f.Path)
		if parent == "." {
			parent = "/"
		}
		childrenOf[parent] = append(childrenOf[parent], components.FolderNode{
			Name: f.Name,
			Path: f.Path,
		})
	}

	// Recursively build tree from root
	var build func(parentPath string) []components.FolderNode
	build = func(parentPath string) []components.FolderNode {
		children := childrenOf[parentPath]
		for i := range children {
			children[i].Children = build(children[i].Path)
		}
		return children
	}

	return build("/")
}

// decodePathSegment URL-decodes a path segment.
func decodePathSegment(s string) (string, error) {
	return strings.ReplaceAll(s, "%20", " "), nil
}

// writeSSE writes a properly formatted SSE event. Multi-line data gets each
// line prefixed with "data: " per the SSE spec.
func writeSSE(w http.ResponseWriter, eventName, data string) {
	fmt.Fprintf(w, "event: %s\n", eventName)
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	q := r.URL.Query().Get("q")

	if q == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		component := partials.SearchResults(nil)
		if err := component.Render(r.Context(), w); err != nil {
			slog.Error("web: failed to render empty search", "error", err)
		}
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := partials.SearchResults(items)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render search results", "error", err)
	}
}

func (h *Handler) handleAgentPage(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	t := T(sess.Locale)

	// List conversations
	convs, err := h.db.ListConversations(r.Context(), sess.SelectedVault, sess.UserID)
	if err != nil {
		slog.Error("web: failed to list conversations", "error", err)
	}

	var agentConvs []pages.AgentConversation
	for _, c := range convs {
		id, err := models.RecordIDString(c.ID)
		if err != nil {
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
		name := vid
		if err == nil && v != nil {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := partials.VersionList(docID, items, currentVersion)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render version list", "error", err)
	}
}

func (h *Handler) handleVersionDiff(w http.ResponseWriter, r *http.Request) {
	docID := r.URL.Query().Get("doc")
	versionStr := r.URL.Query().Get("v")
	currentStr := r.URL.Query().Get("current")

	if docID == "" || versionStr == "" || currentStr == "" {
		http.Error(w, "doc, v, and current parameters required", http.StatusBadRequest)
		return
	}

	var oldVersion, newVersion int
	if _, err := fmt.Sscanf(versionStr, "%d", &oldVersion); err != nil {
		http.Error(w, "invalid version number", http.StatusBadRequest)
		return
	}
	if _, err := fmt.Sscanf(currentStr, "%d", &newVersion); err != nil {
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	component := partials.DiffView(oldVersion, newVersion, lines)
	if err := component.Render(r.Context(), w); err != nil {
		slog.Error("web: failed to render diff view", "error", err)
	}
}

func (h *Handler) handleVaultSwitch(w http.ResponseWriter, r *http.Request) {
	sess := SessionFromContext(r.Context())
	vaultID := r.FormValue("vault")

	if vaultID == "" {
		http.Error(w, "vault parameter required", http.StatusBadRequest)
		return
	}

	// Verify user has access
	hasAccess := false
	for _, v := range sess.VaultAccess {
		if v == vaultID {
			hasAccess = true
			break
		}
	}
	if !hasAccess {
		http.Error(w, "unauthorized vault", http.StatusForbidden)
		return
	}

	sess.SelectedVault = vaultID
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

	sess.Locale = locale

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

	sess.Theme = theme

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
		return c.Value
	}
	return "system"
}
