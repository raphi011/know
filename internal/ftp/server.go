// Package ftp implements a minimal FTP server backed by Know's document service.
// Authentication uses the same token scheme as WebDAV (password = API token).
// Only passive mode (PASV/EPSV) is supported for data transfers.
package ftp

import (
	"context"
	"log/slog"
	"net"
	"sync"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/vault"
)

// Server is an FTP server that serves Know documents.
type Server struct {
	listener   net.Listener
	dbClient   *db.Client
	docService *file.Service
	vaultSvc   *vault.Service
	noAuth     bool

	// pasvMin and pasvMax define the passive port range.
	pasvMin int
	pasvMax int

	quit chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

// NewServer creates a new FTP server.
func NewServer(
	ln net.Listener,
	dbClient *db.Client,
	docService *file.Service,
	vaultSvc *vault.Service,
	noAuth bool,
	pasvMin, pasvMax int,
) *Server {
	return &Server{
		listener:   ln,
		dbClient:   dbClient,
		docService: docService,
		vaultSvc:   vaultSvc,
		noAuth:     noAuth,
		pasvMin:    pasvMin,
		pasvMax:    pasvMax,
		quit:       make(chan struct{}),
	}
}

// Serve accepts FTP connections until Shutdown is called.
func (s *Server) Serve() {
	slog.Info("ftp: listening", "addr", s.listener.Addr())
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				slog.Error("ftp: accept error", "error", err)
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			sess := newSession(conn, s.dbClient, s.docService, s.vaultSvc, s.noAuth, s.pasvMin, s.pasvMax)
			sess.run()
		}()
	}
}

// Shutdown gracefully stops the FTP server. Safe to call multiple times.
func (s *Server) Shutdown(_ context.Context) {
	s.once.Do(func() {
		close(s.quit)
		if err := s.listener.Close(); err != nil {
			slog.Warn("ftp: listener close error", "error", err)
		}
		s.wg.Wait()
		slog.Info("ftp: stopped")
	})
}
