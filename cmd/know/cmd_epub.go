package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/spf13/cobra"
)

var (
	epubAPI     *apiFlags
	epubVaultID *string
	epubPath    string
	epubTitle   string
	epubAuthor  string
	epubOutput  string
)

var epubCmd = &cobra.Command{
	Use:   "epub",
	Short: "Export a document or folder as an EPUB file",
	Long: `Creates an .epub file from a single document or all markdown files in a folder.

Examples:
  know epub --vault default --path /books/my-book.md
  know epub --vault default --path /books/ --title "My Book" --author "Me"
  know epub --vault default --path /notes/ -o notes.epub`,
	RunE: runEpub,
}

func init() {
	epubAPI = addAPIFlags(epubCmd)
	epubVaultID = addVaultFlag(epubCmd, epubAPI)
	epubCmd.Flags().StringVar(&epubPath, "path", "", "document or folder path to export (required)")
	epubCmd.Flags().StringVar(&epubTitle, "title", "", "EPUB title (default: auto-detected from document/folder)")
	epubCmd.Flags().StringVar(&epubAuthor, "author", "", "EPUB author (default: Know)")
	epubCmd.Flags().StringVarP(&epubOutput, "output", "o", "", "output file path (default: <title>.epub)")
	epubCmd.MarkFlagRequired("path")
}

func runEpub(cmd *cobra.Command, args []string) error {
	if epubOutput == "" {
		// Derive output filename from title or path.
		name := epubTitle
		if name == "" {
			name = path.Base(strings.TrimSuffix(epubPath, "/"))
		}
		if name == "" || name == "." || name == "/" {
			name = "export"
		}
		epubOutput = name + ".epub"
	}

	client := epubAPI.newClient()

	n, err := client.DownloadExportEPUB(cmd.Context(), *epubVaultID, epubPath, epubTitle, epubAuthor, epubOutput)
	if err != nil {
		return fmt.Errorf("epub export: %w", err)
	}

	fmt.Printf("EPUB saved to %s (%s)\n", epubOutput, formatBytes(n))
	return nil
}
