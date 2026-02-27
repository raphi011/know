package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	scrapeVaultID string
	scrapeLabels  []string
	scrapeDryRun  bool
	scrapeForce   bool
	scrapeSource  string
)

var scrapeCmd = &cobra.Command{
	Use:   "scrape <directory>",
	Short: "Ingest Markdown files into a vault via the API",
	Long: `Walk a directory for .md files and create/update documents in a vault.

Unchanged files are skipped by comparing content hashes, unless --force is set.
Each file goes through the full document pipeline (parse, embed, link, chunk).

Examples:
  knowhow-v2 scrape ./docs --vault <id>
  knowhow-v2 scrape ./notes --vault <id> --labels personal,notes
  knowhow-v2 scrape ./wiki --vault <id> --dry-run
  knowhow-v2 scrape ./docs --vault <id> --force`,
	Args: cobra.ExactArgs(1),
	RunE: runScrape,
}

func init() {
	scrapeCmd.Flags().StringVar(&scrapeVaultID, "vault", "", "vault ID (required)")
	scrapeCmd.Flags().StringSliceVarP(&scrapeLabels, "labels", "l", nil, "labels to include in document path metadata")
	scrapeCmd.Flags().BoolVar(&scrapeDryRun, "dry-run", false, "show what would be ingested without changes")
	scrapeCmd.Flags().BoolVar(&scrapeForce, "force", false, "re-ingest all files (ignore content hash)")
	scrapeCmd.Flags().StringVar(&scrapeSource, "source", "scrape", "document source tag")
	_ = scrapeCmd.MarkFlagRequired("vault")
}

func runScrape(cmd *cobra.Command, args []string) error {
	if err := requireToken(); err != nil {
		return err
	}

	dirPath := args[0]
	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %s", dirPath)
	}

	client := newGQLClient(apiURL, apiToken)

	// 1. Collect local markdown files
	files, err := collectMarkdownFiles(dirPath)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("No Markdown files found")
		return nil
	}
	fmt.Printf("Found %d Markdown files\n", len(files))

	// 2. Read content and compute hashes
	type localFile struct {
		relPath string // vault-relative path, e.g. /docs/readme.md
		content string
		hash    string
	}

	var localFiles []localFile
	for _, absPath := range files {
		content, err := os.ReadFile(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skip %s: %v\n", absPath, err)
			continue
		}

		rel, err := filepath.Rel(dirPath, absPath)
		if err != nil {
			rel = absPath
		}
		// Normalize to vault path: /relative/path.md
		vaultPath := "/" + filepath.ToSlash(rel)

		h := sha256.Sum256(content)
		localFiles = append(localFiles, localFile{
			relPath: vaultPath,
			content: string(content),
			hash:    hex.EncodeToString(h[:]),
		})
	}

	// 3. For each file, check existing hash and decide whether to ingest
	var created, updated, skipped, errCount int

	for _, lf := range localFiles {
		if scrapeDryRun {
			fmt.Printf("  [dry-run] %s\n", lf.relPath)
			created++
			continue
		}

		// Check existing document's content hash (unless --force)
		if !scrapeForce {
			existing, err := getDocumentHash(client, scrapeVaultID, lf.relPath)
			if err != nil {
				// Document might not exist — that's fine, we'll create it
				fmt.Fprintf(os.Stderr, "Warning: hash check for %s: %v\n", lf.relPath, err)
			} else if existing == lf.hash {
				skipped++
				continue
			}
		}

		// Create/update document via GraphQL
		isNew, err := upsertDocument(client, scrapeVaultID, lf.relPath, lf.content, scrapeSource)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", lf.relPath, err)
			errCount++
			continue
		}

		if isNew {
			fmt.Printf("  + %s\n", lf.relPath)
			created++
		} else {
			fmt.Printf("  ~ %s\n", lf.relPath)
			updated++
		}
	}

	if scrapeDryRun {
		fmt.Printf("\nDry run: %d files would be ingested\n", created)
		return nil
	}

	fmt.Printf("\nDone: %d created, %d updated, %d unchanged, %d errors\n", created, updated, skipped, errCount)
	return nil
}

// getDocumentHash queries the existing document's contentHash.
// Returns empty string if document doesn't exist.
func getDocumentHash(client *gqlClient, vaultID, path string) (string, error) {
	query := `query($vaultId: ID!, $path: String!) {
		document(vaultId: $vaultId, path: $path) {
			contentHash
		}
	}`

	var resp struct {
		Document *struct {
			ContentHash *string `json:"contentHash"`
		} `json:"document"`
	}

	if err := client.do(query, map[string]any{
		"vaultId": vaultID,
		"path":    path,
	}, &resp); err != nil {
		return "", err
	}

	if resp.Document == nil || resp.Document.ContentHash == nil {
		return "", nil
	}
	return *resp.Document.ContentHash, nil
}

// upsertDocument creates or updates a document. Returns true if newly created.
func upsertDocument(client *gqlClient, vaultID, path, content, source string) (bool, error) {
	query := `mutation($vaultId: ID!, $file: FileInput!, $source: String) {
		createDocument(vaultId: $vaultId, file: $file, source: $source) {
			id
			createdAt
			updatedAt
		}
	}`

	var resp struct {
		CreateDocument struct {
			ID        string `json:"id"`
			CreatedAt string `json:"createdAt"`
			UpdatedAt string `json:"updatedAt"`
		} `json:"createDocument"`
	}

	if err := client.do(query, map[string]any{
		"vaultId": vaultID,
		"file": map[string]any{
			"path":    path,
			"content": content,
		},
		"source": source,
	}, &resp); err != nil {
		return false, err
	}

	// If createdAt == updatedAt, it's a new document
	return resp.CreateDocument.CreatedAt == resp.CreateDocument.UpdatedAt, nil
}

// collectMarkdownFiles walks a directory recursively for .md/.markdown files.
func collectMarkdownFiles(dirPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}
	return files, nil
}
