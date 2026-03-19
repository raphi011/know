package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/models"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

func TestTokenToResponse(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	later := now.Add(24 * time.Hour)

	token := models.APIToken{
		ID:        surrealmodels.RecordID{Table: "api_token", ID: "abc123"},
		User:      surrealmodels.RecordID{Table: "user", ID: "admin"},
		TokenHash: "secret-hash-should-not-appear",
		Name:      "my-token",
		LastUsed:  &now,
		ExpiresAt: &later,
		CreatedAt: now,
	}

	resp := tokenToResponse(token, slog.Default())

	if resp.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got %q", resp.ID)
	}
	if resp.Name != "my-token" {
		t.Errorf("expected Name 'my-token', got %q", resp.Name)
	}
	if resp.LastUsed == nil || !resp.LastUsed.Equal(now) {
		t.Errorf("expected LastUsed %v, got %v", now, resp.LastUsed)
	}
	if resp.ExpiresAt == nil || !resp.ExpiresAt.Equal(later) {
		t.Errorf("expected ExpiresAt %v, got %v", later, resp.ExpiresAt)
	}
	if resp.CreatedAt != now {
		t.Errorf("expected CreatedAt %v, got %v", now, resp.CreatedAt)
	}

	// Verify token hash is NOT in JSON output
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "secret-hash") {
		t.Error("TokenResponse JSON contains token_hash — must never expose it")
	}
	if strings.Contains(string(b), "token_hash") {
		t.Error("TokenResponse JSON contains token_hash field")
	}
}

func TestTokenToResponse_noOptionalFields(t *testing.T) {
	token := models.APIToken{
		ID:        surrealmodels.RecordID{Table: "api_token", ID: "xyz"},
		User:      surrealmodels.RecordID{Table: "user", ID: "admin"},
		Name:      "no-optionals",
		CreatedAt: time.Now(),
	}

	resp := tokenToResponse(token, slog.Default())

	if resp.LastUsed != nil {
		t.Error("expected LastUsed to be nil")
	}
	if resp.ExpiresAt != nil {
		t.Error("expected ExpiresAt to be nil")
	}

	// Verify omitempty works
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "last_used") {
		t.Error("expected last_used to be omitted from JSON")
	}
}

func TestListTokens_unauthorized(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tokens", nil)
	rec := httptest.NewRecorder()

	// No auth context injected — should return 401
	s.listTokens(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestCreateToken_unauthorized(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", body)
	rec := httptest.NewRecorder()

	s.createToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestCreateToken_missingName(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", body)
	ctx := auth.WithAuth(req.Context(), auth.AuthContext{UserID: "admin"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.createToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	detail, _ := resp["detail"].(string)
	if !strings.Contains(detail, "name") {
		t.Errorf("expected error detail about name, got %q", detail)
	}
}

func TestCreateToken_invalidExpiresInDays(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"name":"test","expires_in_days":-5}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", body)
	ctx := auth.WithAuth(req.Context(), auth.AuthContext{UserID: "admin"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.createToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreateToken_invalidBody(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{invalid json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens", body)
	ctx := auth.WithAuth(req.Context(), auth.AuthContext{UserID: "admin"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.createToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteToken_unauthorized(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/abc", nil)
	rec := httptest.NewRecorder()

	s.deleteToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestDeleteToken_missingID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tokens/", nil)
	ctx := auth.WithAuth(req.Context(), auth.AuthContext{UserID: "admin"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.deleteToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRotateToken_unauthorized(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens/abc/rotate", nil)
	rec := httptest.NewRecorder()

	s.rotateToken(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRotateToken_missingID(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tokens//rotate", nil)
	ctx := auth.WithAuth(req.Context(), auth.AuthContext{UserID: "admin"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.rotateToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreateTokenResponse_containsRawToken(t *testing.T) {
	resp := CreateTokenResponse{
		RawToken: "kh_abc123",
		Token: TokenResponse{
			ID:        "tok1",
			Name:      "my-token",
			CreatedAt: time.Now(),
		},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded["raw_token"] != "kh_abc123" {
		t.Errorf("expected raw_token 'kh_abc123', got %v", decoded["raw_token"])
	}

	tokenMap, ok := decoded["token"].(map[string]any)
	if !ok {
		t.Fatal("expected token to be an object")
	}
	if tokenMap["name"] != "my-token" {
		t.Errorf("expected token.name 'my-token', got %v", tokenMap["name"])
	}
}
