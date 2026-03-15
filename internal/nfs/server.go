package nfs

import (
	"context"
	"log/slog"
	"net"
	"sync"

	billy "github.com/go-git/go-billy/v5"
	nfs "github.com/willscott/go-nfs"
	nfshelper "github.com/willscott/go-nfs/helpers"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/vault"
)

// handleCacheSize is the number of NFS file handles to cache.
const handleCacheSize = 1024

// Server is an NFSv3 server that serves know documents.
type Server struct {
	listener net.Listener
	handler  nfs.Handler
	quit     chan struct{}
	once     sync.Once
}

// NewServer creates a new NFS server. The server always uses null auth
// (no per-user authentication) since NFSv3 AUTH_UNIX doesn't map to
// Know tokens. Bind the listener to localhost for safety.
func NewServer(
	ln net.Listener,
	dbClient *db.Client,
	docService *document.Service,
	vaultSvc *vault.Service,
) *Server {
	// Build an AuthContext granting system admin access since NFS
	// has no per-user auth — vault visibility is controlled at mount time.
	ac := auth.AuthContext{IsSystemAdmin: true}

	logger := slog.Default().With("component", "nfs")

	fs := NewFS(dbClient, docService, vaultSvc, ac, logger)
	handler := newHandler(fs)
	cached := nfshelper.NewCachingHandler(handler, handleCacheSize)

	return &Server{
		listener: ln,
		handler:  cached,
		quit:     make(chan struct{}),
	}
}

// Serve accepts NFS connections until Shutdown is called.
func (s *Server) Serve() {
	if err := nfs.Serve(s.listener, s.handler); err != nil {
		select {
		case <-s.quit:
			return
		default:
			slog.Error("nfs: serve error", "error", err)
		}
	}
}

// Shutdown gracefully stops the NFS server. Safe to call multiple times.
func (s *Server) Shutdown(_ context.Context) {
	s.once.Do(func() {
		close(s.quit)
		if err := s.listener.Close(); err != nil {
			slog.Warn("nfs: listener close error", "error", err)
		}
		slog.Info("nfs: stopped")
	})
}

// handler implements nfs.Handler for Know's virtual filesystem.
type handler struct {
	fs billy.Filesystem
}

func newHandler(fs billy.Filesystem) *handler {
	return &handler{fs: fs}
}

// Mount handles NFS mount requests. Returns the filesystem unconditionally
// (localhost-only server, auth is not checked).
func (h *handler) Mount(ctx context.Context, conn net.Conn, req nfs.MountRequest) (nfs.MountStatus, billy.Filesystem, []nfs.AuthFlavor) {
	logutil.FromCtx(ctx).Info("nfs: mount request", "path", req.Dirpath, "remote", conn.RemoteAddr())
	return nfs.MountStatusOk, h.fs, []nfs.AuthFlavor{nfs.AuthFlavorNull}
}

// Change returns nil — file attribute changes (chmod, chown, chtimes) are not supported.
func (h *handler) Change(_ billy.Filesystem) billy.Change {
	return nil
}

// FSStat provides filesystem statistics (stubbed).
func (h *handler) FSStat(_ context.Context, _ billy.Filesystem, s *nfs.FSStat) error {
	return nil
}

// ToHandle, FromHandle, InvalidateHandle, HandleLimit are handled by CachingHandler wrapper.
func (h *handler) ToHandle(_ billy.Filesystem, _ []string) []byte        { return []byte{} }
func (h *handler) FromHandle([]byte) (billy.Filesystem, []string, error) { return nil, nil, nil }
func (h *handler) InvalidateHandle(billy.Filesystem, []byte) error       { return nil }
func (h *handler) HandleLimit() int                                      { return -1 }
