package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestBM25ChunkSearch(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	goDoc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/search-go.md", Title: "Go Programming",
		Content: "---\ntitle: Go\n---\nGo is a statically typed language", ContentBody: "Go is a statically typed language",
		Source: models.SourceManual, Labels: []string{"programming"},
	})
	if err != nil {
		t.Fatalf("CreateDocument go failed: %v", err)
	}
	goDocID := models.MustRecordIDString(goDoc.ID)

	pyDoc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/search-python.md", Title: "Python Programming",
		Content: "---\ntitle: Python\n---\nPython is a dynamic language", ContentBody: "Python is a dynamic language",
		Source: models.SourceManual, Labels: []string{"programming"},
	})
	if err != nil {
		t.Fatalf("CreateDocument python failed: %v", err)
	}
	pyDocID := models.MustRecordIDString(pyDoc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: goDocID, Content: "Go is a statically typed language", Position: 0},
		{DocumentID: pyDocID, Content: "Python is a dynamic language", Position: 0},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	results, err := testDB.BM25ChunkSearch(ctx, "Go statically typed", SearchFilter{
		VaultID: vaultID,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("BM25ChunkSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("BM25ChunkSearch should return results for 'Go statically typed'")
	}
}

func TestSearchWithLabelFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	webDoc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/label-a.md", Title: "Web Doc",
		Content: "Web frameworks are great", ContentBody: "Web frameworks are great",
		Source: models.SourceManual, Labels: []string{"web"},
	})
	if err != nil {
		t.Fatalf("CreateDocument web failed: %v", err)
	}
	webDocID := models.MustRecordIDString(webDoc.ID)

	cliDoc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/label-b.md", Title: "CLI Doc",
		Content: "CLI tools are useful frameworks", ContentBody: "CLI tools are useful frameworks",
		Source: models.SourceManual, Labels: []string{"cli"},
	})
	if err != nil {
		t.Fatalf("CreateDocument cli failed: %v", err)
	}
	cliDocID := models.MustRecordIDString(cliDoc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: webDocID, Content: "Web frameworks are great", Position: 0},
		{DocumentID: cliDocID, Content: "CLI tools are useful frameworks", Position: 0},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	results, err := testDB.BM25ChunkSearch(ctx, "frameworks", SearchFilter{
		VaultID: vaultID,
		Labels:  []string{"web"},
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("BM25ChunkSearch with labels failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("BM25ChunkSearch with label filter should return results")
	}
	for _, r := range results {
		docID, err := models.RecordIDString(r.Document)
		if err != nil {
			t.Fatalf("extract doc ID: %v", err)
		}
		if docID != webDocID {
			t.Errorf("result chunk belongs to doc %q, expected %q (web doc only)", docID, webDocID)
		}
	}
}

func TestSearchWithFolderFilter(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	guidesDoc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/guides/setup.md", Title: "Setup Guide",
		Content: "Install the software first", ContentBody: "Install the software first",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument guides failed: %v", err)
	}
	guidesDocID := models.MustRecordIDString(guidesDoc.ID)

	notesDoc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/notes/install.md", Title: "Install Notes",
		Content: "Notes about installing software", ContentBody: "Notes about installing software",
		Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument notes failed: %v", err)
	}
	notesDocID := models.MustRecordIDString(notesDoc.ID)

	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: guidesDocID, Content: "Install the software first", Position: 0},
		{DocumentID: notesDocID, Content: "Notes about installing software", Position: 0},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	folder := "/guides/"
	results, err := testDB.BM25ChunkSearch(ctx, "software", SearchFilter{
		VaultID: vaultID,
		Folder:  &folder,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("BM25ChunkSearch with folder failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("BM25ChunkSearch with folder filter should return results")
	}
	for _, r := range results {
		docID, err := models.RecordIDString(r.Document)
		if err != nil {
			t.Fatalf("extract doc ID: %v", err)
		}
		if docID != guidesDocID {
			t.Errorf("result chunk belongs to doc %q, expected %q (guides doc only)", docID, guidesDocID)
		}
	}
}

func TestHybridSearch(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID:     vaultID,
		Path:        "/hybrid-search-" + suffix + ".md",
		Title:       "Hybrid Search Doc",
		Content:     "hybrid search content for testing",
		ContentBody: "hybrid search content for testing",
		Source:      models.SourceManual,
		Labels:      []string{"test"},
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	docID := models.MustRecordIDString(doc.ID)

	embedding := dummyEmbedding()
	if err := testDB.CreateChunks(ctx, []models.ChunkInput{
		{DocumentID: docID, Content: "hybrid search content for testing", Position: 0, Embedding: embedding},
	}); err != nil {
		t.Fatalf("CreateChunks failed: %v", err)
	}

	results, err := testDB.HybridSearch(ctx, "hybrid search", embedding, SearchFilter{
		VaultID: vaultID,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("HybridSearch failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("HybridSearch should return results")
	}

	r := results[0]
	if r.DocTitle != "Hybrid Search Doc" {
		t.Errorf("expected DocTitle 'Hybrid Search Doc', got %q", r.DocTitle)
	}
	if r.DocPath != "/hybrid-search-"+suffix+".md" {
		t.Errorf("expected DocPath '/hybrid-search-%s.md', got %q", suffix, r.DocPath)
	}
	if len(r.DocLabels) != 1 || r.DocLabels[0] != "test" {
		t.Errorf("expected DocLabels [test], got %v", r.DocLabels)
	}
}

func TestGetDocumentsByIDs(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	var docIDs []string
	for i, path := range []string{"/byids-" + suffix + "/a.md", "/byids-" + suffix + "/b.md"} {
		doc, err := testDB.CreateDocument(ctx, models.DocumentInput{
			VaultID:     vaultID,
			Path:        path,
			Title:       fmt.Sprintf("ByIDs Doc %d", i),
			Content:     "content " + path,
			ContentBody: "content " + path,
			Source:      models.SourceManual,
			Labels:      []string{},
		})
		if err != nil {
			t.Fatalf("CreateDocument %s failed: %v", path, err)
		}
		docIDs = append(docIDs, models.MustRecordIDString(doc.ID))
	}

	docs, err := testDB.GetDocumentsByIDs(ctx, docIDs)
	if err != nil {
		t.Fatalf("GetDocumentsByIDs failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(docs))
	}
}
