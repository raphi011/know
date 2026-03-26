// Package sshd provides an SSH server with SFTP subsystem for accessing
// know documents. Authentication uses know API tokens as SSH passwords.
package sshd

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"crypto/x509"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/metrics"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/vault"
)

// Server is an SSH server that serves SFTP for know documents.
type Server struct {
	listener   net.Listener
	sshConfig  *ssh.ServerConfig
	dbClient   *db.Client
	docService *file.Service
	vaultSvc   *vault.Service
	noAuth     bool
	metrics    *metrics.Metrics

	wg        sync.WaitGroup
	quit      chan struct{}
	closeOnce sync.Once
}

// NewServer creates a new SSH server. If hostKeyPath is empty, an Ed25519 key
// is auto-generated at ~/.know/host_key.
func NewServer(
	ln net.Listener,
	dbClient *db.Client,
	docService *file.Service,
	vaultSvc *vault.Service,
	hostKeyPath string,
	noAuth bool,
	m *metrics.Metrics,
) (*Server, error) {
	s := &Server{
		listener:   ln,
		dbClient:   dbClient,
		docService: docService,
		vaultSvc:   vaultSvc,
		noAuth:     noAuth,
		metrics:    m,
		quit:       make(chan struct{}),
	}

	sshCfg := &ssh.ServerConfig{
		PasswordCallback: s.passwordCallback,
		MaxAuthTries:     3,
	}

	hostKey, err := loadOrGenerateHostKey(hostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("host key: %w", err)
	}

	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}

	sshCfg.AddHostKey(signer)
	slog.Info("SSH host key loaded", "fingerprint", ssh.FingerprintSHA256(signer.PublicKey()))

	s.sshConfig = sshCfg
	return s, nil
}

// Serve accepts SSH connections until Shutdown is called.
func (s *Server) Serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				slog.Warn("ssh: accept error", "error", err)
				continue
			}
		}

		s.wg.Go(func() {
			s.handleConnection(conn)
		})
	}
}

// Shutdown gracefully stops the SSH server. Safe to call multiple times.
func (s *Server) Shutdown(ctx context.Context) {
	s.closeOnce.Do(func() {
		close(s.quit)
		if err := s.listener.Close(); err != nil {
			slog.Warn("ssh: listener close error", "error", err)
		}
	})

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("ssh: all connections closed")
	case <-ctx.Done():
		slog.Warn("ssh: shutdown deadline exceeded, some connections may be abandoned")
	}
}

// passwordCallback validates the API token provided as SSH password.
func (s *Server) passwordCallback(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	ac, err := auth.Authenticate(context.Background(), s.dbClient, string(password), s.noAuth)
	if err != nil {
		slog.Warn("ssh: auth failed", "user", conn.User(), "remote", conn.RemoteAddr(), "error", err)
		auth.AuditLog(context.Background(), auth.AuditFailure,
			slog.String("protocol", "ssh"),
			slog.String("ip", conn.RemoteAddr().String()),
		)
		s.metrics.RecordSSHConnection("failure")
		return nil, fmt.Errorf("authentication failed")
	}

	s.metrics.RecordSSHConnection("success")
	auth.AuditLog(context.Background(), auth.AuditSuccess,
		slog.String("protocol", "ssh"),
		slog.String("user_id", ac.UserID),
		slog.String("ip", conn.RemoteAddr().String()),
	)

	// Serialize vault permissions as "vaultID:role,vaultID:role,..."
	var parts []string
	for _, vp := range ac.Vaults {
		parts = append(parts, vp.VaultID+":"+string(vp.Role))
	}

	isAdmin := "false"
	if ac.IsSystemAdmin {
		isAdmin = "true"
	}

	return &ssh.Permissions{
		Extensions: map[string]string{
			"user_id":         ac.UserID,
			"vault_access":    strings.Join(parts, ","),
			"is_system_admin": isAdmin,
		},
	}, nil
}

// handleConnection performs the SSH handshake and handles channels.
func (s *Server) handleConnection(netConn net.Conn) {
	sshConn, chans, reqs, err := ssh.NewServerConn(netConn, s.sshConfig)
	if err != nil {
		slog.Warn("ssh: handshake failed", "remote", netConn.RemoteAddr(), "error", err)
		return
	}
	defer sshConn.Close()

	slog.Info("ssh: connection established", "user", sshConn.User(), "remote", sshConn.RemoteAddr())

	// Discard global requests
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "unknown channel type")
			continue
		}

		channel, requests, err := newChan.Accept()
		if err != nil {
			slog.Warn("ssh: channel accept failed", "error", err)
			continue
		}

		go s.handleSession(channel, requests, sshConn.Permissions)
	}
}

// handleSession waits for the "sftp" subsystem request and starts the SFTP server.
func (s *Server) handleSession(channel ssh.Channel, requests <-chan *ssh.Request, perms *ssh.Permissions) {
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "subsystem":
			// Payload is a uint32 length-prefixed string
			if len(req.Payload) < 4 {
				if err := req.Reply(false, nil); err != nil {
					slog.Debug("ssh: reply failed", "type", req.Type, "error", err)
				}
				continue
			}
			subsystem := string(req.Payload[4:])

			if subsystem != "sftp" {
				slog.Debug("ssh: unsupported subsystem", "subsystem", subsystem)
				if err := req.Reply(false, nil); err != nil {
					slog.Debug("ssh: reply failed", "type", req.Type, "error", err)
				}
				continue
			}

			if err := req.Reply(true, nil); err != nil {
				slog.Debug("ssh: reply failed", "type", req.Type, "error", err)
				return
			}

			ac := authContextFromPermissions(perms)
			h := newHandler(s.dbClient, s.docService, s.vaultSvc, ac, s.metrics)

			srv := sftp.NewRequestServer(channel, h.Handlers())
			if err := srv.Serve(); err != nil {
				if !errors.Is(err, sftp.ErrSSHFxConnectionLost) {
					slog.Warn("sftp: server error", "error", err)
				}
			}
			return

		default:
			if req.WantReply {
				if err := req.Reply(false, nil); err != nil {
					slog.Debug("ssh: reply failed", "type", req.Type, "error", err)
				}
			}
		}
	}
}

// authContextFromPermissions reconstructs an AuthContext from SSH Permissions.Extensions.
func authContextFromPermissions(perms *ssh.Permissions) auth.AuthContext {
	if perms == nil {
		return auth.AuthContext{}
	}

	var vaults []models.VaultPermission
	if va := perms.Extensions["vault_access"]; va != "" {
		for entry := range strings.SplitSeq(va, ",") {
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				continue
			}
			role, err := models.ParseVaultRole(parts[1])
			if err != nil {
				slog.Warn("ssh: invalid vault role in permissions", "entry", entry, "error", err)
				continue
			}
			vaults = append(vaults, models.VaultPermission{
				VaultID: parts[0],
				Role:    role,
			})
		}
	}

	return auth.AuthContext{
		UserID:        perms.Extensions["user_id"],
		IsSystemAdmin: perms.Extensions["is_system_admin"] == "true",
		Vaults:        vaults,
	}
}

// loadOrGenerateHostKey loads an SSH host key from path, or auto-generates
// an Ed25519 key if path is empty.
func loadOrGenerateHostKey(path string) (ed25519.PrivateKey, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("user home dir: %w", err)
		}
		path = filepath.Join(home, ".know", "host_key")
	}

	// Try loading existing key
	data, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("invalid PEM in %s", path)
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse host key: %w", err)
		}
		ed25519Key, ok := key.(ed25519.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("host key is not Ed25519")
		}
		slog.Info("ssh: loaded host key", "path", path)
		return ed25519Key, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read host key: %w", err)
	}

	// Generate new Ed25519 key
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}

	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8,
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, fmt.Errorf("write host key: %w", err)
	}

	slog.Info("ssh: generated new host key", "path", path)
	return priv, nil
}
