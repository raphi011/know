package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/raphi011/know/internal/document"
	"github.com/raphi011/know/internal/parser"
	"github.com/raphi011/know/internal/search"
	"github.com/raphi011/know/internal/tools"
)

// setupExecutor creates a vault and returns a tools.Executor wired to the test DB.
func setupExecutor(t *testing.T, suffix string) (*tools.Executor, string) {
	t.Helper()
	ctx := context.Background()
	vaultID, _ := setupVault(t, ctx, "exec-"+suffix+"-"+fmt.Sprint(time.Now().UnixNano()))

	searchSvc := search.NewService(testDB, nil)
	docSvc := document.NewService(testDB, nil, parser.DefaultChunkConfig(), document.VersionConfig{CoalesceMinutes: 10, RetentionCount: 50}, nil, 0)

	return &tools.Executor{
		DB:         testDB,
		Search:     searchSvc,
		DocService: docSvc,
	}, vaultID
}

func TestToolsExecutor_CreateAndReadDocument(t *testing.T) {
	exec, vaultID := setupExecutor(t, "create-read")
	ctx := context.Background()

	// Create
	result, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/docs/hello.md","content":"# Hello\n\nWorld."}`)
	if err != nil {
		t.Fatalf("create_document: %v", err)
	}
	if !strings.Contains(result, "Document created") {
		t.Errorf("unexpected create result: %s", result)
	}

	// Read
	result, meta, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/docs/hello.md"}`)
	if err != nil {
		t.Fatalf("read_document: %v", err)
	}
	if !strings.Contains(result, "Hello") {
		t.Errorf("read_document missing title: %s", result)
	}
	if !strings.Contains(result, "World.") {
		t.Errorf("read_document missing content: %s", result)
	}
	if !strings.Contains(result, "Content-Hash:") {
		t.Errorf("read_document missing content hash: %s", result)
	}
	if meta == nil || meta.DocumentPath == nil || *meta.DocumentPath != "/docs/hello.md" {
		t.Error("meta should contain document path")
	}
}

func TestToolsExecutor_CreateDocument_AlreadyExists(t *testing.T) {
	exec, vaultID := setupExecutor(t, "already-exists")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/dup.md","content":"# First"}`)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, _, err = exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/dup.md","content":"# Second"}`)
	if err == nil {
		t.Fatal("expected error for duplicate create")
	}
	var toolErr *tools.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if !strings.Contains(toolErr.Message, "already exists") {
		t.Errorf("unexpected error message: %s", toolErr.Message)
	}
}

func TestToolsExecutor_EditDocument(t *testing.T) {
	exec, vaultID := setupExecutor(t, "edit")
	ctx := context.Background()

	// Create
	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/edit.md","content":"# Original\n\nContent."}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Read to get hash
	result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/edit.md"}`)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	hash := extractContentHash(t, result)

	// Edit with correct hash
	args := fmt.Sprintf(`{"path":"/edit.md","content":"# Updated\n\nNew content.","expected_hash":"%s"}`, hash)
	result, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document", args)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(result, "Document updated") {
		t.Errorf("unexpected edit result: %s", result)
	}

	// Verify update
	result, _, err = exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/edit.md"}`)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !strings.Contains(result, "New content.") {
		t.Errorf("content not updated: %s", result)
	}
}

func TestToolsExecutor_EditDocument_HashMismatch(t *testing.T) {
	exec, vaultID := setupExecutor(t, "hash-mismatch")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/mismatch.md","content":"# Doc"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document", `{"path":"/mismatch.md","content":"# New","expected_hash":"badhash"}`)
	if err == nil {
		t.Fatal("expected error for hash mismatch")
	}
	var toolErr *tools.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if !strings.Contains(toolErr.Message, "changed since you read it") {
		t.Errorf("unexpected error: %s", toolErr.Message)
	}
}

func TestToolsExecutor_EditDocumentSection(t *testing.T) {
	t.Run("Replace", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-replace")
		ctx := context.Background()

		content := "# Doc\n\nPreamble.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B."
		args := fmt.Sprintf(`{"path":"/replace.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/replace.md","operation":"replace","heading":"Section A","content":"Replaced A."}`)
		if err != nil {
			t.Fatalf("replace: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/replace.md"}`)
		if err != nil {
			t.Fatalf("read after replace: %v", err)
		}
		if !strings.Contains(result, "Replaced A.") {
			t.Errorf("Section A not replaced: %s", result)
		}
		if !strings.Contains(result, "Content B.") {
			t.Errorf("Section B should be intact: %s", result)
		}
		if !strings.Contains(result, "Preamble.") {
			t.Errorf("Preamble should be intact: %s", result)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-delete")
		ctx := context.Background()

		content := "# Doc\n\nPreamble.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B."
		args := fmt.Sprintf(`{"path":"/delete.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/delete.md","operation":"delete","heading":"Section A"}`)
		if err != nil {
			t.Fatalf("delete: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/delete.md"}`)
		if err != nil {
			t.Fatalf("read after delete: %v", err)
		}
		if strings.Contains(result, "Content A.") {
			t.Errorf("Section A content should be deleted: %s", result)
		}
		if !strings.Contains(result, "Content B.") {
			t.Errorf("Section B should be intact: %s", result)
		}
	})

	t.Run("InsertAfter", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-insert-after")
		ctx := context.Background()

		content := "# Doc\n\nPreamble.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B."
		args := fmt.Sprintf(`{"path":"/insert-after.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/insert-after.md","operation":"insert_after","heading":"Section A","new_heading":"Section A.5","new_level":2,"content":"Inserted after A."}`)
		if err != nil {
			t.Fatalf("insert_after: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/insert-after.md"}`)
		if err != nil {
			t.Fatalf("read after insert_after: %v", err)
		}
		if !strings.Contains(result, "Section A.5") {
			t.Errorf("inserted section heading missing: %s", result)
		}
		if !strings.Contains(result, "Inserted after A.") {
			t.Errorf("inserted section content missing: %s", result)
		}
		// Original sections should be intact
		if !strings.Contains(result, "Content A.") || !strings.Contains(result, "Content B.") {
			t.Errorf("original sections should be intact: %s", result)
		}
	})

	t.Run("InsertBefore", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-insert-before")
		ctx := context.Background()

		content := "# Doc\n\nPreamble.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B."
		args := fmt.Sprintf(`{"path":"/insert-before.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/insert-before.md","operation":"insert_before","heading":"Section B","new_heading":"Section A.5","new_level":2,"content":"Inserted before B."}`)
		if err != nil {
			t.Fatalf("insert_before: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/insert-before.md"}`)
		if err != nil {
			t.Fatalf("read after insert_before: %v", err)
		}
		if !strings.Contains(result, "Section A.5") {
			t.Errorf("inserted section heading missing: %s", result)
		}
		if !strings.Contains(result, "Inserted before B.") {
			t.Errorf("inserted section content missing: %s", result)
		}
		if !strings.Contains(result, "Content A.") || !strings.Contains(result, "Content B.") {
			t.Errorf("original sections should be intact: %s", result)
		}
	})

	t.Run("Append", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-append")
		ctx := context.Background()

		content := "# Doc\n\nPreamble.\n\n## Section A\n\nContent A."
		args := fmt.Sprintf(`{"path":"/append.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/append.md","operation":"append","new_heading":"Appendix","new_level":2,"content":"Appended content."}`)
		if err != nil {
			t.Fatalf("append: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/append.md"}`)
		if err != nil {
			t.Fatalf("read after append: %v", err)
		}
		if !strings.Contains(result, "Appendix") {
			t.Errorf("appended section heading missing: %s", result)
		}
		if !strings.Contains(result, "Appended content.") {
			t.Errorf("appended section content missing: %s", result)
		}
		if !strings.Contains(result, "Content A.") {
			t.Errorf("original content should be intact: %s", result)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-404")
		ctx := context.Background()

		_, _, err := exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/ghost.md","operation":"replace","heading":"X","content":"Y"}`)
		if err == nil {
			t.Fatal("expected error for nonexistent doc")
		}
		var toolErr *tools.ToolError
		if !errors.As(err, &toolErr) {
			t.Fatalf("expected ToolError, got %T: %v", err, err)
		}
		if !strings.Contains(toolErr.Message, "not found") {
			t.Errorf("unexpected error: %s", toolErr.Message)
		}
	})

	t.Run("InvalidOperation", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-invalid-op")
		ctx := context.Background()

		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/invalid-op.md","content":"# Doc\n\nContent."}`)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/invalid-op.md","operation":"bogus","heading":"Doc","content":"X"}`)
		if err == nil {
			t.Fatal("expected error for invalid operation")
		}
		var toolErr *tools.ToolError
		if !errors.As(err, &toolErr) {
			t.Fatalf("expected ToolError, got %T: %v", err, err)
		}
		if !strings.Contains(toolErr.Message, "unknown operation") {
			t.Errorf("unexpected error: %s", toolErr.Message)
		}
	})

	t.Run("HashMismatch", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-hash")
		ctx := context.Background()

		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", `{"path":"/hash-section.md","content":"# Doc\n\n## Sec\n\nBody."}`)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/hash-section.md","operation":"replace","heading":"Sec","content":"New.","expected_hash":"badhash"}`)
		if err == nil {
			t.Fatal("expected error for hash mismatch")
		}
		var toolErr *tools.ToolError
		if !errors.As(err, &toolErr) {
			t.Fatalf("expected ToolError, got %T: %v", err, err)
		}
		if !strings.Contains(toolErr.Message, "changed since you read it") {
			t.Errorf("unexpected error: %s", toolErr.Message)
		}
	})

	t.Run("ReplacePreamble", func(t *testing.T) {
		// Document with actual preamble text before the first heading.
		exec, vaultID := setupExecutor(t, "section-preamble")
		ctx := context.Background()

		content := "Old preamble text.\n\n## Section A\n\nContent A."
		args := fmt.Sprintf(`{"path":"/preamble.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		// Replace preamble (empty heading = preamble)
		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/preamble.md","operation":"replace","heading":"","content":"New preamble text."}`)
		if err != nil {
			t.Fatalf("replace preamble: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/preamble.md"}`)
		if err != nil {
			t.Fatalf("read after preamble replace: %v", err)
		}
		if !strings.Contains(result, "New preamble text.") {
			t.Errorf("preamble not replaced: %s", result)
		}
		if strings.Contains(result, "Old preamble text.") {
			t.Errorf("old preamble should be gone: %s", result)
		}
		if !strings.Contains(result, "Content A.") {
			t.Errorf("Section A should be intact: %s", result)
		}
	})

	t.Run("ReplacePreamble_NoPreambleExists", func(t *testing.T) {
		// When a document starts with a heading (no preamble), targeting
		// heading="" for replace should return an error, not silently corrupt
		// the document.
		exec, vaultID := setupExecutor(t, "section-no-preamble")
		ctx := context.Background()

		content := "# Doc\n\n## Section A\n\nContent A."
		args := fmt.Sprintf(`{"path":"/no-preamble.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		// Attempt to replace preamble on a doc with no preamble → should error
		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/no-preamble.md","operation":"replace","heading":"","content":"Injected."}`)
		if err == nil {
			t.Fatal("expected error when replacing preamble on doc without preamble")
		}
		if !strings.Contains(err.Error(), "preamble section not found") {
			t.Errorf("expected 'preamble section not found' error, got: %v", err)
		}

		// Verify the document was NOT modified
		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/no-preamble.md"}`)
		if err != nil {
			t.Fatalf("read after no-preamble: %v", err)
		}
		if strings.Contains(result, "Injected.") {
			t.Errorf("document should not contain injected content: %s", result)
		}
		if !strings.Contains(result, "## Section A") {
			t.Errorf("original content should be intact: %s", result)
		}
	})

	t.Run("SequentialEdits", func(t *testing.T) {
		// Verifies that multiple sequential section edits don't corrupt the doc.
		exec, vaultID := setupExecutor(t, "section-sequential")
		ctx := context.Background()

		content := "# Doc\n\nPreamble.\n\n## Alpha\n\nAlpha content.\n\n## Beta\n\nBeta content.\n\n## Gamma\n\nGamma content."
		args := fmt.Sprintf(`{"path":"/sequential.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		// Edit 1: Replace Alpha
		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/sequential.md","operation":"replace","heading":"Alpha","content":"Alpha v2."}`)
		if err != nil {
			t.Fatalf("edit 1: %v", err)
		}

		// Edit 2: Delete Beta
		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/sequential.md","operation":"delete","heading":"Beta"}`)
		if err != nil {
			t.Fatalf("edit 2: %v", err)
		}

		// Edit 3: Append new section
		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/sequential.md","operation":"append","new_heading":"Delta","new_level":2,"content":"Delta content."}`)
		if err != nil {
			t.Fatalf("edit 3: %v", err)
		}

		// Verify final state
		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/sequential.md"}`)
		if err != nil {
			t.Fatalf("read after sequential edits: %v", err)
		}
		if !strings.Contains(result, "Preamble.") {
			t.Errorf("preamble should be intact: %s", result)
		}
		if !strings.Contains(result, "Alpha v2.") {
			t.Errorf("Alpha should be v2: %s", result)
		}
		if strings.Contains(result, "Beta content.") {
			t.Errorf("Beta should be deleted: %s", result)
		}
		if !strings.Contains(result, "Gamma content.") {
			t.Errorf("Gamma should be intact: %s", result)
		}
		if !strings.Contains(result, "Delta content.") {
			t.Errorf("Delta should be appended: %s", result)
		}
	})

	t.Run("InsertAfterPreservesOrder", func(t *testing.T) {
		// Verifies insert_after places new section between the target and next section.
		exec, vaultID := setupExecutor(t, "section-order")
		ctx := context.Background()

		content := "# Doc\n\n## First\n\n1st content.\n\n## Third\n\n3rd content."
		args := fmt.Sprintf(`{"path":"/order.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/order.md","operation":"insert_after","heading":"First","new_heading":"Second","new_level":2,"content":"2nd content."}`)
		if err != nil {
			t.Fatalf("insert_after: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/order.md"}`)
		if err != nil {
			t.Fatalf("read after insert_after: %v", err)
		}
		// Verify order: First appears before Second, Second appears before Third
		firstIdx := strings.Index(result, "1st content.")
		secondIdx := strings.Index(result, "2nd content.")
		thirdIdx := strings.Index(result, "3rd content.")
		if firstIdx == -1 || secondIdx == -1 || thirdIdx == -1 {
			t.Fatalf("missing sections in result: %s", result)
		}
		if !(firstIdx < secondIdx && secondIdx < thirdIdx) {
			t.Errorf("sections out of order: First@%d Second@%d Third@%d\n%s",
				firstIdx, secondIdx, thirdIdx, result)
		}
	})

	t.Run("NonexistentHeading", func(t *testing.T) {
		exec, vaultID := setupExecutor(t, "section-no-heading")
		ctx := context.Background()

		content := "# Doc\n\n## Existing\n\nContent."
		args := fmt.Sprintf(`{"path":"/no-heading.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section",
			`{"path":"/no-heading.md","operation":"replace","heading":"Nonexistent","content":"X"}`)
		if err == nil {
			t.Fatal("expected error for nonexistent heading")
		}
		var toolErr *tools.ToolError
		if !errors.As(err, &toolErr) {
			t.Fatalf("expected ToolError, got %T: %v", err, err)
		}
	})

	t.Run("WithExpectedHash", func(t *testing.T) {
		// Verifies that section edit works with correct expected_hash.
		exec, vaultID := setupExecutor(t, "section-with-hash")
		ctx := context.Background()

		content := "# Doc\n\n## Sec\n\nOriginal."
		args := fmt.Sprintf(`{"path":"/with-hash.md","content":%q}`, content)
		_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document", args)
		if err != nil {
			t.Fatalf("create: %v", err)
		}

		// Read to get hash
		readResult, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/with-hash.md"}`)
		if err != nil {
			t.Fatalf("read for hash: %v", err)
		}
		hash := extractContentHash(t, readResult)

		// Edit with correct hash
		editArgs := fmt.Sprintf(`{"path":"/with-hash.md","operation":"replace","heading":"Sec","content":"Updated.","expected_hash":"%s"}`, hash)
		_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document_section", editArgs)
		if err != nil {
			t.Fatalf("edit with hash: %v", err)
		}

		result, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/with-hash.md"}`)
		if err != nil {
			t.Fatalf("read after hash edit: %v", err)
		}
		if !strings.Contains(result, "Updated.") {
			t.Errorf("section not updated: %s", result)
		}
		if strings.Contains(result, "Original.") {
			t.Errorf("old content should be gone: %s", result)
		}
	})
}

func TestToolsExecutor_CreateMemory(t *testing.T) {
	exec, vaultID := setupExecutor(t, "memory")
	ctx := context.Background()

	result, meta, err := exec.ExecuteTool(ctx, vaultID, "create_memory",
		`{"title":"My Test Memory","content":"Remember this.","labels":["test"],"project":"test-project"}`)
	if err != nil {
		t.Fatalf("create_memory: %v", err)
	}
	if !strings.Contains(result, "/memories/test-project/") {
		t.Errorf("expected /memories/test-project/ path in result: %s", result)
	}
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(result, today) {
		t.Errorf("expected date %s in path: %s", today, result)
	}
	if meta == nil || meta.DocumentPath == nil {
		t.Fatal("expected meta with document path")
	}

	// Verify the memory document has the "memory" label
	readResult, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", fmt.Sprintf(`{"path":%q}`, *meta.DocumentPath))
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	if !strings.Contains(readResult, "Remember this.") {
		t.Errorf("memory content missing: %s", readResult)
	}
}

func TestToolsExecutor_Search(t *testing.T) {
	exec, vaultID := setupExecutor(t, "search")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/searchable.md","content":"# Quantum Computing\n\nQuantum computing uses qubits."}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/other.md","content":"# Cooking\n\nHow to bake bread."}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := exec.DocService.ProcessAllPending(ctx); err != nil {
		t.Fatalf("process pending: %v", err)
	}

	result, _, err := exec.ExecuteTool(ctx, vaultID, "search", `{"query":"quantum"}`)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(result, "Quantum") {
		t.Errorf("search result missing expected doc: %s", result)
	}
}

func TestToolsExecutor_ListLabels(t *testing.T) {
	exec, vaultID := setupExecutor(t, "labels")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/labeled.md","content":"---\nlabels: [go, backend]\n---\n# Doc"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, meta, err := exec.ExecuteTool(ctx, vaultID, "list_labels", "{}")
	if err != nil {
		t.Fatalf("list_labels: %v", err)
	}
	if !strings.Contains(result, "go") || !strings.Contains(result, "backend") {
		t.Errorf("missing labels in result: %s", result)
	}
	if meta == nil || meta.ResultCount == nil || *meta.ResultCount < 2 {
		t.Error("expected at least 2 labels in meta")
	}
}

func TestToolsExecutor_ListFolders(t *testing.T) {
	exec, vaultID := setupExecutor(t, "folders")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/guides/getting-started.md","content":"# Guide"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/notes/daily.md","content":"# Notes"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, _, err := exec.ExecuteTool(ctx, vaultID, "list_folders", `{}`)
	if err != nil {
		t.Fatalf("list_folders: %v", err)
	}
	if !strings.Contains(result, "/guides") {
		t.Errorf("missing /guides folder: %s", result)
	}
	if !strings.Contains(result, "/notes") {
		t.Errorf("missing /notes folder: %s", result)
	}
}

func TestToolsExecutor_ListFolderContents(t *testing.T) {
	exec, vaultID := setupExecutor(t, "folder-contents")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/proj/readme.md","content":"# Readme"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, _, err = exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/proj/sub/deep.md","content":"# Deep"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	result, _, err := exec.ExecuteTool(ctx, vaultID, "list_folder_contents", `{"folder":"/proj"}`)
	if err != nil {
		t.Fatalf("list_folder_contents: %v", err)
	}
	if !strings.Contains(result, "readme.md") {
		t.Errorf("missing readme.md: %s", result)
	}
	if !strings.Contains(result, "sub") {
		t.Errorf("missing sub folder: %s", result)
	}
	// deep.md should NOT appear (it's not an immediate child of /proj)
	if strings.Contains(result, "deep.md") {
		t.Errorf("deep.md should not be an immediate child of /proj: %s", result)
	}
}

func TestToolsExecutor_GetDocumentVersions(t *testing.T) {
	exec, vaultID := setupExecutor(t, "versions")
	ctx := context.Background()

	// Create
	_, _, err := exec.ExecuteTool(ctx, vaultID, "create_document",
		`{"path":"/versioned.md","content":"# V1"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update twice (via edit_document without hash check for simplicity).
	// Note: version coalescing (CoalesceMinutes: 10) may merge rapid updates
	// into a single version snapshot, so we only assert >= 1 version.
	_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document",
		`{"path":"/versioned.md","content":"# V2"}`)
	if err != nil {
		t.Fatalf("edit 1: %v", err)
	}
	_, _, err = exec.ExecuteTool(ctx, vaultID, "edit_document",
		`{"path":"/versioned.md","content":"# V3"}`)
	if err != nil {
		t.Fatalf("edit 2: %v", err)
	}

	result, meta, err := exec.ExecuteTool(ctx, vaultID, "get_document_versions",
		`{"path":"/versioned.md"}`)
	if err != nil {
		t.Fatalf("get_document_versions: %v", err)
	}
	if !strings.Contains(result, "Total versions:") {
		t.Errorf("missing version count: %s", result)
	}
	if meta == nil || meta.ResultCount == nil || *meta.ResultCount < 1 {
		t.Errorf("expected at least 1 version, got %v", meta)
	}
	if !strings.Contains(result, "Version") {
		t.Errorf("expected version details in result: %s", result)
	}
}

func TestToolsExecutor_ReadDocument_NotFound(t *testing.T) {
	exec, vaultID := setupExecutor(t, "read-404")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "read_document", `{"path":"/nonexistent.md"}`)
	if err == nil {
		t.Fatal("expected error for reading nonexistent document")
	}
	var toolErr *tools.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if !strings.Contains(toolErr.Message, "document not found") {
		t.Errorf("expected 'document not found' in error, got: %s", toolErr.Message)
	}
}

func TestToolsExecutor_EditDocument_NotFound(t *testing.T) {
	exec, vaultID := setupExecutor(t, "edit-404")
	ctx := context.Background()

	_, _, err := exec.ExecuteTool(ctx, vaultID, "edit_document",
		`{"path":"/ghost.md","content":"# New"}`)
	if err == nil {
		t.Fatal("expected error for editing nonexistent document")
	}
	var toolErr *tools.ToolError
	if !errors.As(err, &toolErr) {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if !strings.Contains(toolErr.Message, "not found") {
		t.Errorf("unexpected error: %s", toolErr.Message)
	}
}

// extractContentHash parses the Content-Hash line from read_document output.
func extractContentHash(t *testing.T, output string) string {
	t.Helper()
	for line := range strings.SplitSeq(output, "\n") {
		if hash, ok := strings.CutPrefix(line, "Content-Hash: "); ok {
			return hash
		}
	}
	t.Fatalf("no Content-Hash found in output:\n%s", output)
	return ""
}
