package api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/epub"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
)

func (s *Server) exportEPUB(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logutil.FromCtx(ctx)

	vaultID := r.URL.Query().Get("vault")
	if vaultID == "" {
		writeError(w, http.StatusBadRequest, "vault query parameter is required")
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeError(w, http.StatusBadRequest, "path query parameter is required")
		return
	}

	if err := auth.RequireVaultRole(ctx, vaultID, models.RoleRead); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}

	dbClient := s.app.DBClient()

	// Determine if path is a file or folder.
	var chapters []epub.Chapter
	f, err := dbClient.GetFileByPath(ctx, vaultID, filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("get file: %v", err))
		return
	}

	if f != nil && !f.IsFolder {
		// Single document mode.
		chapters = append(chapters, epub.Chapter{
			Title:   f.Title,
			Content: f.Content,
			Path:    f.Path,
		})
	} else if f == nil && path.Ext(filePath) != "" {
		// Specific file requested but not found.
		writeError(w, http.StatusNotFound, fmt.Sprintf("document not found: %s", filePath))
		return
	} else {
		// Folder mode — list markdown files under this path.
		folderPath := filePath
		if !strings.HasSuffix(folderPath, "/") {
			folderPath += "/"
		}

		mimeType := "text/markdown"
		const pageSize = 1000
		for offset := 0; ; offset += pageSize {
			batch, err := dbClient.ListFiles(ctx, db.ListFilesFilter{
				VaultID:  vaultID,
				Folder:   &folderPath,
				MimeType: &mimeType,
				OrderBy:  db.OrderByPathAsc,
				Limit:    pageSize,
				Offset:   offset,
			})
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("list files: %v", err))
				return
			}
			for _, file := range batch {
				chapters = append(chapters, epub.Chapter{
					Title:   file.Title,
					Content: file.Content,
					Path:    file.Path,
				})
			}
			if len(batch) < pageSize {
				break
			}
		}

		if len(chapters) == 0 {
			writeError(w, http.StatusNotFound, "no markdown files found at path")
			return
		}
	}

	// Collect unique image paths from all chapters, tracking the chapter directory
	// for relative path resolution.
	imagePaths := make(map[string]bool)
	chapterDirs := make(map[string]string) // relative imagePath → chapter dir
	for _, ch := range chapters {
		for _, imgPath := range epub.ExtractImagePaths(ch.Content) {
			if imagePaths[imgPath] {
				continue
			}
			imagePaths[imgPath] = true
			if !path.IsAbs(imgPath) && !strings.HasPrefix(imgPath, "http://") && !strings.HasPrefix(imgPath, "https://") {
				chapterDirs[imgPath] = path.Dir(ch.Path)
			}
		}
	}

	// Resolve images.
	images, err := s.resolveImages(ctx, vaultID, imagePaths, chapterDirs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("resolve images: %v", err))
		return
	}

	// Determine title and author.
	title := r.URL.Query().Get("title")
	if title == "" {
		if len(chapters) == 1 {
			title = chapters[0].Title
		} else {
			title = path.Base(strings.TrimSuffix(filePath, "/"))
		}
	}
	if title == "" {
		title = "Export"
	}

	author := r.URL.Query().Get("author")

	opts := epub.Options{
		Title:  title,
		Author: author,
	}

	data, err := epub.Generate(opts, chapters, images)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("generate epub: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": sanitizeFilename(title) + ".epub"}))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	if _, err := w.Write(data); err != nil {
		logger.Warn("failed to write epub response", "error", err)
	}
}

const maxImageSize = 10 << 20 // 10MB per image

func (s *Server) resolveImages(ctx context.Context, vaultID string, imagePaths map[string]bool, chapterDirs map[string]string) ([]epub.Image, error) {
	logger := logutil.FromCtx(ctx)
	dbClient := s.app.DBClient()
	var images []epub.Image
	var failCount int

	for imgPath := range imagePaths {
		if strings.HasPrefix(imgPath, "https://") {
			img, err := fetchExternalImage(ctx, imgPath, len(images))
			if err != nil {
				logger.Warn("failed to fetch external image", "url", imgPath, "error", err)
				failCount++
				continue
			}
			images = append(images, *img)
			continue
		}
		if strings.HasPrefix(imgPath, "http://") {
			logger.Warn("skipping insecure image URL", "url", imgPath)
			failCount++
			continue
		}

		// Resolve vault path.
		vaultPath := imgPath
		if !path.IsAbs(vaultPath) {
			if dir, ok := chapterDirs[imgPath]; ok {
				vaultPath = path.Join(dir, vaultPath)
			}
		}

		f, err := dbClient.GetFileByPath(ctx, vaultID, vaultPath)
		if err != nil {
			logger.Warn("failed to look up image", "path", vaultPath, "error", err)
			failCount++
			continue
		}
		if f == nil {
			logger.Warn("image not found in vault", "path", vaultPath)
			failCount++
			continue
		}

		var data []byte
		if f.ContentHash != nil {
			rc, err := s.app.BlobStore().Get(ctx, *f.ContentHash)
			if err != nil {
				logger.Warn("failed to read image blob", "path", vaultPath, "error", err)
				failCount++
				continue
			}
			data, err = io.ReadAll(io.LimitReader(rc, maxImageSize))
			if closeErr := rc.Close(); closeErr != nil {
				logger.Warn("failed to close image blob reader", "path", vaultPath, "error", closeErr)
			}
			if err != nil {
				logger.Warn("failed to read image data", "path", vaultPath, "error", err)
				failCount++
				continue
			}
		} else {
			data = []byte(f.Content)
		}

		filename := fmt.Sprintf("img%04d%s", len(images), path.Ext(f.Path))
		images = append(images, epub.Image{
			VaultPath: imgPath,
			Filename:  filename,
			Data:      data,
			MimeType:  f.MimeType,
		})
	}

	if failCount > 0 && len(images) == 0 {
		return nil, fmt.Errorf("all %d images failed to resolve", failCount)
	}

	return images, nil
}

func fetchExternalImage(ctx context.Context, imgURL string, counter int) (*epub.Image, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imgURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}

	ext := extFromMIME(mimeType)
	filename := fmt.Sprintf("img%04d%s", counter, ext)

	return &epub.Image{
		VaultPath: imgURL,
		Filename:  filename,
		Data:      data,
		MimeType:  mimeType,
	}, nil
}

func extFromMIME(mimeType string) string {
	mediaType, _, _ := mime.ParseMediaType(mimeType)
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	default:
		return ".png"
	}
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", `"`, "", "<", "", ">", "",
		"|", "", "?", "", "*", "", "\r", "", "\n", "", ";", "",
	)
	return replacer.Replace(name)
}
