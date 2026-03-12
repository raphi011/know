// Package remote manages connections to other knowhow servers for federation.
package remote

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/raphi011/knowhow/internal/db"
	"github.com/raphi011/knowhow/internal/logutil"
	"github.com/raphi011/knowhow/internal/models"
)

// RemoteVault represents a vault on a remote server with its namespaced name.
type RemoteVault struct {
	RemoteName string // e.g. "home"
	VaultID    string // vault ID on the remote server
	VaultName  string // vault name on the remote server
	Namespace  string // e.g. "home/default"
}

// Service manages remote server configurations and clients.
type Service struct {
	db      *db.Client
	clients sync.Map // remoteName -> *apiclient.Client

	cacheMu      sync.RWMutex
	vaultCache   []RemoteVault
	cacheExpires time.Time
}

const vaultCacheTTL = 60 * time.Second

// NewService creates a new remote service.
func NewService(db *db.Client) *Service {
	return &Service{db: db}
}

// Add registers a new remote server.
func (s *Service) Add(ctx context.Context, userID string, input models.RemoteInput) (*models.Remote, error) {
	r, err := s.db.CreateRemote(ctx, userID, input)
	if err != nil {
		return nil, fmt.Errorf("add remote: %w", err)
	}
	// Pre-create the client
	s.clients.Store(input.Name, apiclient.New(input.URL, input.Token))
	// Invalidate vault cache
	s.invalidateCache()
	return r, nil
}

// Remove deletes a remote server configuration.
func (s *Service) Remove(ctx context.Context, name string) error {
	deleted, err := s.db.DeleteRemote(ctx, name)
	if err != nil {
		return fmt.Errorf("remove remote: %w", err)
	}
	if !deleted {
		return fmt.Errorf("remote %q: %w", name, db.ErrNotFound)
	}
	s.clients.Delete(name)
	s.invalidateCache()
	return nil
}

// List returns all configured remotes.
func (s *Service) List(ctx context.Context) ([]models.Remote, error) {
	return s.db.ListRemotes(ctx)
}

// ListRemoteVaults discovers vaults on all configured remotes.
// Results are cached for 60 seconds. Unreachable remotes are skipped with a warning.
func (s *Service) ListRemoteVaults(ctx context.Context) ([]RemoteVault, error) {
	s.cacheMu.RLock()
	if time.Now().Before(s.cacheExpires) && s.vaultCache != nil {
		cached := s.vaultCache
		s.cacheMu.RUnlock()
		return cached, nil
	}
	s.cacheMu.RUnlock()

	logger := logutil.FromCtx(ctx)

	remotes, err := s.db.ListRemotes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list remote vaults: %w", err)
	}

	var result []RemoteVault

	for _, r := range remotes {
		client := s.clientFor(r.Name, r.URL, r.Token)
		vaults, err := client.ListVaults(ctx)
		if err != nil {
			logger.Warn("remote unreachable, skipping", "remote", r.Name, "url", r.URL, "error", err)
			continue
		}
		for _, v := range vaults {
			result = append(result, RemoteVault{
				RemoteName: r.Name,
				VaultID:    v.ID,
				VaultName:  v.Name,
				Namespace:  r.Name + "/" + v.Name,
			})
		}
	}

	s.cacheMu.Lock()
	s.vaultCache = result
	s.cacheExpires = time.Now().Add(vaultCacheTTL)
	s.cacheMu.Unlock()

	return result, nil
}

// ClientFor returns an apiclient for the named remote.
// Returns an error if the remote is not configured.
func (s *Service) ClientFor(ctx context.Context, remoteName string) (*apiclient.Client, error) {
	if c, ok := s.clients.Load(remoteName); ok {
		return c.(*apiclient.Client), nil
	}

	// Lazy-load from DB
	r, err := s.db.GetRemoteByName(ctx, remoteName)
	if err != nil {
		return nil, fmt.Errorf("client for %q: %w", remoteName, err)
	}
	if r == nil {
		return nil, fmt.Errorf("remote %q not found", remoteName)
	}

	client := s.clientFor(r.Name, r.URL, r.Token)
	return client, nil
}

func (s *Service) clientFor(name, url, token string) *apiclient.Client {
	c, _ := s.clients.LoadOrStore(name, apiclient.New(url, token))
	return c.(*apiclient.Client)
}

func (s *Service) invalidateCache() {
	s.cacheMu.Lock()
	s.vaultCache = nil
	s.cacheExpires = time.Time{}
	s.cacheMu.Unlock()
}
