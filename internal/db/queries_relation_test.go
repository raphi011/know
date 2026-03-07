package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/raphi011/knowhow/internal/models"
)

func TestCreateRelation(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	docA, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/rel-a-" + suffix + ".md", Title: "Rel Doc A",
		Content: "content a", ContentBody: "content a", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument A failed: %v", err)
	}
	docB, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/rel-b-" + suffix + ".md", Title: "Rel Doc B",
		Content: "content b", ContentBody: "content b", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument B failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	rel, err := testDB.CreateRelation(ctx, models.DocRelationInput{
		FromDocID: docAID,
		ToDocID:   docBID,
		RelType:   "relates_to",
		Source:    "api",
	})
	if err != nil {
		t.Fatalf("CreateRelation failed: %v", err)
	}
	if rel.RelType != "relates_to" {
		t.Errorf("Expected rel_type 'relates_to', got %q", rel.RelType)
	}
	if rel.Source != "api" {
		t.Errorf("Expected source 'api', got %q", rel.Source)
	}
}

func TestGetRelations(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	docA, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/getrel-a-" + suffix + ".md", Title: "GetRel A",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument A failed: %v", err)
	}
	docB, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/getrel-b-" + suffix + ".md", Title: "GetRel B",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument B failed: %v", err)
	}
	docC, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/getrel-c-" + suffix + ".md", Title: "GetRel C",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument C failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)
	docCID := models.MustRecordIDString(docC.ID)

	// Create two relations from A
	_, err = testDB.CreateRelation(ctx, models.DocRelationInput{
		FromDocID: docAID, ToDocID: docBID, RelType: "relates_to", Source: "api",
	})
	if err != nil {
		t.Fatalf("CreateRelation A->B failed: %v", err)
	}
	_, err = testDB.CreateRelation(ctx, models.DocRelationInput{
		FromDocID: docAID, ToDocID: docCID, RelType: "relates_to", Source: "api",
	})
	if err != nil {
		t.Fatalf("CreateRelation A->C failed: %v", err)
	}

	rels, err := testDB.GetRelations(ctx, docAID)
	if err != nil {
		t.Fatalf("GetRelations failed: %v", err)
	}
	if len(rels) != 2 {
		t.Errorf("Expected 2 relations, got %d", len(rels))
	}
}

func TestGetRelationByID(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	docA, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/relbyid-a-" + suffix + ".md", Title: "RelByID A",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument A failed: %v", err)
	}
	docB, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/relbyid-b-" + suffix + ".md", Title: "RelByID B",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument B failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	rel, err := testDB.CreateRelation(ctx, models.DocRelationInput{
		FromDocID: docAID, ToDocID: docBID, RelType: "relates_to", Source: "api",
	})
	if err != nil {
		t.Fatalf("CreateRelation failed: %v", err)
	}
	relID := models.MustRecordIDString(rel.ID)

	fetched, err := testDB.GetRelationByID(ctx, relID)
	if err != nil {
		t.Fatalf("GetRelationByID failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetRelationByID returned nil for existing relation")
	}
	if fetched.RelType != "relates_to" {
		t.Errorf("Expected rel_type 'relates_to', got %q", fetched.RelType)
	}

	// Nonexistent relation should return nil
	notFound, err := testDB.GetRelationByID(ctx, "doc_relation:nonexistent_"+suffix)
	if err != nil {
		t.Fatalf("GetRelationByID nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("Expected nil for nonexistent relation")
	}
}

func TestDeleteRelation(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)
	vault := createTestVault(t, ctx, userID)
	vaultID := models.MustRecordIDString(vault.ID)

	suffix := fmt.Sprint(time.Now().UnixNano())
	docA, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/delrel-a-" + suffix + ".md", Title: "DelRel A",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument A failed: %v", err)
	}
	docB, err := testDB.CreateDocument(ctx, models.DocumentInput{
		VaultID: vaultID, Path: "/delrel-b-" + suffix + ".md", Title: "DelRel B",
		Content: "content", ContentBody: "content", Source: models.SourceManual, Labels: []string{},
	})
	if err != nil {
		t.Fatalf("CreateDocument B failed: %v", err)
	}
	docAID := models.MustRecordIDString(docA.ID)
	docBID := models.MustRecordIDString(docB.ID)

	rel, err := testDB.CreateRelation(ctx, models.DocRelationInput{
		FromDocID: docAID, ToDocID: docBID, RelType: "relates_to", Source: "api",
	})
	if err != nil {
		t.Fatalf("CreateRelation failed: %v", err)
	}
	relID := models.MustRecordIDString(rel.ID)

	err = testDB.DeleteRelation(ctx, relID)
	if err != nil {
		t.Fatalf("DeleteRelation failed: %v", err)
	}

	gone, err := testDB.GetRelationByID(ctx, relID)
	if err != nil {
		t.Fatalf("GetRelationByID after delete error: %v", err)
	}
	if gone != nil {
		t.Error("Relation should be nil after delete")
	}
}
