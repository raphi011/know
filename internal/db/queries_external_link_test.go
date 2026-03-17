package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateExternalLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-create-" + suffix + ".md", Title: "Ext Create",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	linkText := "Go Blog"
	links := []ExternalLinkInput{
		{Hostname: "go.dev", URLPath: "/blog", FullURL: "https://go.dev/blog", LinkText: &linkText},
		{Hostname: "go.dev", URLPath: "/doc", FullURL: "https://go.dev/doc"},
		{Hostname: "github.com", URLPath: "/golang/go", FullURL: "https://github.com/golang/go"},
	}

	if err := testDB.CreateExternalLinks(ctx, fileID, vaultID, links); err != nil {
		t.Fatalf("CreateExternalLinks failed: %v", err)
	}

	listed, err := testDB.ListExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID, Limit: 50})
	if err != nil {
		t.Fatalf("ListExternalLinks failed: %v", err)
	}
	if len(listed) != 3 {
		t.Errorf("expected 3 external links, got %d", len(listed))
	}
}

func TestCreateExternalLinks_Empty(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-empty-" + suffix + ".md", Title: "Ext Empty",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateExternalLinks(ctx, fileID, vaultID, []ExternalLinkInput{}); err != nil {
		t.Errorf("CreateExternalLinks with empty slice should not error, got: %v", err)
	}
}

func TestDeleteExternalLinksByFile(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	fileA, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-del-a-" + suffix + ".md", Title: "Ext Del A",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile A failed: %v", err)
	}
	fileB, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-del-b-" + suffix + ".md", Title: "Ext Del B",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile B failed: %v", err)
	}
	fileAID := models.MustRecordIDString(fileA.ID)
	fileBID := models.MustRecordIDString(fileB.ID)

	if err := testDB.CreateExternalLinks(ctx, fileAID, vaultID, []ExternalLinkInput{
		{Hostname: "example.com", URLPath: "/a", FullURL: "https://example.com/a"},
	}); err != nil {
		t.Fatalf("CreateExternalLinks A failed: %v", err)
	}
	if err := testDB.CreateExternalLinks(ctx, fileBID, vaultID, []ExternalLinkInput{
		{Hostname: "example.com", URLPath: "/b", FullURL: "https://example.com/b"},
		{Hostname: "example.com", URLPath: "/b2", FullURL: "https://example.com/b2"},
	}); err != nil {
		t.Fatalf("CreateExternalLinks B failed: %v", err)
	}

	if err := testDB.DeleteExternalLinksByFile(ctx, fileAID); err != nil {
		t.Fatalf("DeleteExternalLinksByFile failed: %v", err)
	}

	// File A's links should be gone
	linksA, err := testDB.ListExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID, FileID: &fileAID, Limit: 50})
	if err != nil {
		t.Fatalf("ListExternalLinks for file A after delete failed: %v", err)
	}
	if len(linksA) != 0 {
		t.Errorf("expected 0 links for file A after delete, got %d", len(linksA))
	}

	// File B's links should remain
	linksB, err := testDB.ListExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID, FileID: &fileBID, Limit: 50})
	if err != nil {
		t.Fatalf("ListExternalLinks for file B failed: %v", err)
	}
	if len(linksB) != 2 {
		t.Errorf("expected 2 links for file B, got %d", len(linksB))
	}
}

func TestGetExternalLinkStats(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-stats-" + suffix + ".md", Title: "Ext Stats",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateExternalLinks(ctx, fileID, vaultID, []ExternalLinkInput{
		{Hostname: "go.dev", URLPath: "/blog", FullURL: "https://go.dev/blog"},
		{Hostname: "go.dev", URLPath: "/doc", FullURL: "https://go.dev/doc"},
		{Hostname: "go.dev", URLPath: "/ref", FullURL: "https://go.dev/ref"},
		{Hostname: "github.com", URLPath: "/a", FullURL: "https://github.com/a"},
	}); err != nil {
		t.Fatalf("CreateExternalLinks failed: %v", err)
	}

	stats, err := testDB.GetExternalLinkStats(ctx, vaultID)
	if err != nil {
		t.Fatalf("GetExternalLinkStats failed: %v", err)
	}

	statsMap := make(map[string]int, len(stats))
	for _, s := range stats {
		statsMap[s.Hostname] = s.Count
	}

	if statsMap["go.dev"] != 3 {
		t.Errorf("expected go.dev count=3, got %d", statsMap["go.dev"])
	}
	if statsMap["github.com"] != 1 {
		t.Errorf("expected github.com count=1, got %d", statsMap["github.com"])
	}

	// Stats should be ordered by count DESC
	if len(stats) >= 2 && stats[0].Count < stats[1].Count {
		t.Errorf("expected stats ordered by count DESC, got %v then %v", stats[0].Count, stats[1].Count)
	}
}

func TestListExternalLinks_HostnameFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-hostname-" + suffix + ".md", Title: "Ext Hostname",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	if err := testDB.CreateExternalLinks(ctx, fileID, vaultID, []ExternalLinkInput{
		{Hostname: "golang.org", URLPath: "/pkg", FullURL: "https://golang.org/pkg"},
		{Hostname: "golang.org", URLPath: "/ref", FullURL: "https://golang.org/ref"},
		{Hostname: "rust-lang.org", URLPath: "/doc", FullURL: "https://rust-lang.org/doc"},
	}); err != nil {
		t.Fatalf("CreateExternalLinks failed: %v", err)
	}

	hostname := "golang.org"
	links, err := testDB.ListExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID, Hostname: &hostname, Limit: 50})
	if err != nil {
		t.Fatalf("ListExternalLinks with hostname filter failed: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links for golang.org, got %d", len(links))
	}
	for _, l := range links {
		if l.Hostname != "golang.org" {
			t.Errorf("expected hostname 'golang.org', got %q", l.Hostname)
		}
	}
}

func TestListExternalLinks_Pagination(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-page-" + suffix + ".md", Title: "Ext Page",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	batch := make([]ExternalLinkInput, 5)
	for i := range 5 {
		batch[i] = ExternalLinkInput{
			Hostname: "paginate.example",
			URLPath:  fmt.Sprintf("/page/%d", i),
			FullURL:  fmt.Sprintf("https://paginate.example/page/%d", i),
		}
	}
	if err := testDB.CreateExternalLinks(ctx, fileID, vaultID, batch); err != nil {
		t.Fatalf("CreateExternalLinks failed: %v", err)
	}

	links, err := testDB.ListExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID, Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListExternalLinks with limit=2 failed: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 links with limit=2, got %d", len(links))
	}
}

func TestCountExternalLinks(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	file, err := testDB.CreateFile(ctx, models.FileInput{
		VaultID: vaultID, Path: "/ext-count-" + suffix + ".md", Title: "Ext Count",
		Content: "content", Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateFile failed: %v", err)
	}
	fileID := models.MustRecordIDString(file.ID)

	batch := []ExternalLinkInput{
		{Hostname: "count.example", URLPath: "/1", FullURL: "https://count.example/1"},
		{Hostname: "count.example", URLPath: "/2", FullURL: "https://count.example/2"},
		{Hostname: "count.example", URLPath: "/3", FullURL: "https://count.example/3"},
	}
	if err := testDB.CreateExternalLinks(ctx, fileID, vaultID, batch); err != nil {
		t.Fatalf("CreateExternalLinks failed: %v", err)
	}

	count, err := testDB.CountExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID})
	if err != nil {
		t.Fatalf("CountExternalLinks failed: %v", err)
	}

	listed, err := testDB.ListExternalLinks(ctx, ExternalLinkFilter{VaultID: vaultID, Limit: 999})
	if err != nil {
		t.Fatalf("ListExternalLinks failed: %v", err)
	}

	if count != len(listed) {
		t.Errorf("CountExternalLinks=%d does not match ListExternalLinks count=%d", count, len(listed))
	}
	if count < 3 {
		t.Errorf("expected at least 3 links, got %d", count)
	}
}
