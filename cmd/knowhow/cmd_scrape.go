package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/raphi011/knowhow/internal/apiclient"
	"github.com/raphi011/knowhow/internal/models"
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
	Short: "Ingest Markdown files and images into a vault via the API",
	Long: `Walk a directory for .md files and images and create/update documents/assets in a vault.

Unchanged files are skipped by comparing content hashes, unless --force is set.
Markdown files go through the full document pipeline (parse, embed, link, chunk).
Image files (PNG, JPEG, GIF, SVG, WebP) are uploaded as binary assets.

Environment variables:
  KNOWHOW_VAULT    vault name (alternative to --vault flag)

Examples:
  knowhow scrape ./docs --vault <id>
  knowhow scrape ./notes --vault <id> --labels personal,notes
  knowhow scrape ./wiki --vault <id> --dry-run
  knowhow scrape ./docs --vault <id> --force`,
	Args: cobra.ExactArgs(1),
	RunE: runScrape,
}

func init() {
	scrapeCmd.Flags().StringVar(&scrapeVaultID, "vault", envOrDefault("KNOWHOW_VAULT", "default"), "vault name (env: KNOWHOW_VAULT)")
	scrapeCmd.Flags().StringSliceVarP(&scrapeLabels, "labels", "l", nil, "labels to include in document path metadata")
	scrapeCmd.Flags().BoolVar(&scrapeDryRun, "dry-run", false, "show what would be ingested without changes")
	scrapeCmd.Flags().BoolVar(&scrapeForce, "force", false, "re-ingest all files (ignore content hash)")
	scrapeCmd.Flags().StringVar(&scrapeSource, "source", "scrape", "document source tag")
}

func runScrape(cmd *cobra.Command, args []string) error {
	dirPath := args[0]
	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %s", dirPath)
	}

	client := apiclient.New(apiURL, apiToken)

	// 1. Collect local files (markdown + images)
	files, err := collectFiles(dirPath)
	if err != nil {
		return fmt.Errorf("scrape: %w", err)
	}
	if len(files) == 0 {
		fmt.Println("No files found")
		return nil
	}

	var mdCount, imgCount int
	for _, f := range files {
		if models.IsImageFile(f) {
			imgCount++
		} else {
			mdCount++
		}
	}
	fmt.Printf("Found %d Markdown files, %d images\n", mdCount, imgCount)

	// 2. Read content and compute hashes
	type localFile struct {
		relPath string // vault-relative path, e.g. /docs/readme.md
		content []byte
		hash    string
		isImage bool
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
		vaultPath := "/" + filepath.ToSlash(rel)

		h := sha256.Sum256(content)
		localFiles = append(localFiles, localFile{
			relPath: vaultPath,
			content: content,
			hash:    hex.EncodeToString(h[:]),
			isImage: models.IsImageFile(absPath),
		})
	}

	// 3. For each file, check existing hash and decide whether to ingest
	var created, updated, skipped, errCount, imagesUploaded int

	for _, lf := range localFiles {
		if scrapeDryRun {
			fmt.Printf("  [dry-run] %s\n", lf.relPath)
			created++
			continue
		}

		if lf.isImage {
			// Check existing asset hash (unless --force)
			if !scrapeForce {
				existing, err := getAssetHash(client, scrapeVaultID, lf.relPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: hash check for %s: %v\n", lf.relPath, err)
				} else if existing == lf.hash {
					skipped++
					continue
				}
			}

			if err := uploadAsset(client, scrapeVaultID, lf.relPath, lf.content); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s: %v\n", lf.relPath, err)
				errCount++
				continue
			}
			fmt.Printf("  + %s (image)\n", lf.relPath)
			imagesUploaded++
			continue
		}

		// Markdown file
		if !scrapeForce {
			existing, err := getDocumentHash(client, scrapeVaultID, lf.relPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: hash check for %s: %v\n", lf.relPath, err)
			} else if existing == lf.hash {
				skipped++
				continue
			}
		}

		isNew, err := upsertDocument(client, scrapeVaultID, lf.relPath, string(lf.content), scrapeSource)
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

	fmt.Printf("\nDone: %d created, %d updated, %d images uploaded, %d unchanged, %d errors\n", created, updated, imagesUploaded, skipped, errCount)
	return nil
}

// getDocumentHash queries the existing document's contentHash via REST API.
// Returns empty string if document doesn't exist.
func getDocumentHash(client *apiclient.Client, vaultID, path string) (string, error) {
	var doc struct {
		ContentHash *string `json:"contentHash"`
	}

	q := url.Values{"vault": {vaultID}, "path": {path}}
	err := client.Get(context.Background(), "/api/documents?"+q.Encode(), &doc)
	if err != nil {
		return "", fmt.Errorf("get hash: %w", err)
	}

	if doc.ContentHash == nil {
		return "", nil
	}
	return *doc.ContentHash, nil
}

// upsertDocument creates or updates a document via REST API. Returns true if newly created.
func upsertDocument(client *apiclient.Client, vaultID, path, content, source string) (bool, error) {
	var resp struct {
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}

	body := map[string]string{
		"vaultId": vaultID,
		"path":    path,
		"content": content,
		"source":  source,
	}

	if err := client.Post(context.Background(), "/api/documents", body, &resp); err != nil {
		return false, fmt.Errorf("upsert: %w", err)
	}

	// If createdAt == updatedAt, it's a new document
	return resp.CreatedAt == resp.UpdatedAt, nil
}

// collectFiles walks a directory recursively for .md/.markdown and image files.
func collectFiles(dirPath string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".md" || ext == ".markdown" || models.IsImageFile(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}
	return files, nil
}

// getAssetHash queries the existing asset's contentHash via REST API.
// Returns empty string if asset doesn't exist.
func getAssetHash(client *apiclient.Client, vaultID, path string) (string, error) {
	var asset struct {
		ContentHash string `json:"contentHash"`
	}

	q := url.Values{"vault": {vaultID}, "path": {path}}
	err := client.Get(context.Background(), "/api/assets/meta?"+q.Encode(), &asset)
	if err != nil {
		return "", fmt.Errorf("get asset hash: %w", err)
	}

	return asset.ContentHash, nil
}

// uploadAsset uploads an image file via multipart POST to the REST API.
func uploadAsset(client *apiclient.Client, vaultID, path string, data []byte) error {
	return client.PostMultipart(
		context.Background(),
		"/api/assets",
		map[string]string{
			"vault": vaultID,
			"path":  path,
		},
		"file",
		filepath.Base(path),
		bytes.NewReader(data),
		nil,
	)
}
