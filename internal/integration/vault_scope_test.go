package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/raphi011/know/internal/api"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/blob"
	"github.com/raphi011/know/internal/file"
	"github.com/raphi011/know/internal/models"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/server"
	"github.com/raphi011/know/internal/vault"
)

func TestVaultScope_NonexistentVaultReturns404(t *testing.T) {
	ctx := context.Background()

	suffix := fmt.Sprint(time.Now().UnixNano())
	_, vaultName, vaultSvc := setupVault(t, ctx, "scope-404-"+suffix)

	blobStore := blob.NewFS(t.TempDir())
	fileSvc := file.NewService(testDB, blobStore, nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)
	app := server.NewForTest(testDB, blobStore, fileSvc, vaultSvc)
	apiSrv := api.NewServer(app)
	mux := http.NewServeMux()
	apiSrv.Register(mux, auth.NoAuthMiddleware, nil)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Access a nonexistent vault
	resp, err := http.Get(srv.URL + "/api/v1/vaults/nonexistent-vault-xyz/labels")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 404 for nonexistent vault, got %d: %s", resp.StatusCode, string(body))
	}

	// Verify the existing vault works
	resp2, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/labels")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200 for existing vault, got %d: %s", resp2.StatusCode, string(body))
	}
}

func TestVaultScope_ForbiddenReturns403(t *testing.T) {
	ctx := context.Background()

	suffix := fmt.Sprint(time.Now().UnixNano())

	// Create a user with access to vault A only
	user, err := testDB.CreateUser(ctx, models.UserInput{Name: "scope-403-user-" + suffix})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := models.MustRecordIDString(user.ID)

	vaultSvc := vault.NewService(testDB)

	// Vault A — user has access
	vA, err := vaultSvc.Create(ctx, userID, models.VaultInput{Name: "scope-403-a-" + suffix})
	if err != nil {
		t.Fatalf("create vault A: %v", err)
	}
	vaultAID := models.MustRecordIDString(vA.ID)
	vaultAName := "scope-403-a-" + suffix

	// Vault B — different user owns it
	user2, err := testDB.CreateUser(ctx, models.UserInput{Name: "scope-403-user2-" + suffix})
	if err != nil {
		t.Fatalf("create user2: %v", err)
	}
	user2ID := models.MustRecordIDString(user2.ID)
	_, err = vaultSvc.Create(ctx, user2ID, models.VaultInput{Name: "scope-403-b-" + suffix})
	if err != nil {
		t.Fatalf("create vault B: %v", err)
	}
	vaultBName := "scope-403-b-" + suffix

	blobStore := blob.NewFS(t.TempDir())
	fileSvc := file.NewService(testDB, blobStore, nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)
	app := server.NewForTest(testDB, blobStore, fileSvc, vaultSvc)
	apiSrv := api.NewServer(app)
	mux := http.NewServeMux()

	// Use a middleware that grants access only to vault A
	scopedAuthMw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithAuth(r.Context(), auth.AuthContext{
				UserID: userID,
				Vaults: []models.VaultPermission{
					{VaultID: vaultAID, Role: models.RoleAdmin},
				},
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	apiSrv.Register(mux, scopedAuthMw, nil)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Vault A should be accessible
	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultAName + "/labels")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authorized vault, got %d", resp.StatusCode)
	}

	// Vault B should be forbidden
	resp2, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultBName + "/labels")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for unauthorized vault, got %d", resp2.StatusCode)
	}
}

func TestVaultScope_InjectsVaultIDIntoContext(t *testing.T) {
	ctx := context.Background()

	suffix := fmt.Sprint(time.Now().UnixNano())
	vaultID, vaultName, vaultSvc := setupVault(t, ctx, "scope-inject-"+suffix)

	blobStore := blob.NewFS(t.TempDir())
	fileSvc := file.NewService(testDB, blobStore, nil, parser.DefaultChunkConfig(), file.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)
	app := server.NewForTest(testDB, blobStore, fileSvc, vaultSvc)
	apiSrv := api.NewServer(app)
	mux := http.NewServeMux()
	apiSrv.Register(mux, auth.NoAuthMiddleware, nil)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Create a document in the vault, then retrieve it — verifies the vault ID
	// was correctly injected and used by the handler.
	_, err := fileSvc.Create(ctx, models.FileInput{
		VaultID: vaultID,
		Path:    "/scope-test.md",
		Content: "# Scope Test",
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	resp, err := http.Get(srv.URL + "/api/v1/vaults/" + vaultName + "/documents?path=/scope-test.md")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["path"] != "/scope-test.md" {
		t.Errorf("expected path=/scope-test.md, got %v", doc["path"])
	}
}
