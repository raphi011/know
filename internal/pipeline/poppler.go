package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/raphi011/know/internal/logutil"
)

// CheckPoppler verifies that pdftoppm and pdftotext are available in PATH.
func CheckPoppler() error {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return fmt.Errorf("pdftoppm not found: %w", err)
	}
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return fmt.Errorf("pdftotext not found: %w", err)
	}
	return nil
}

// PageCount returns the number of pages in a PDF via pdfinfo.
func PageCount(ctx context.Context, pdfPath string) (int, error) {
	logger := logutil.FromCtx(ctx)
	start := time.Now()
	defer func() {
		logger.Debug("pdfinfo complete", "path", pdfPath, "duration_ms", time.Since(start).Milliseconds())
	}()

	out, err := exec.CommandContext(ctx, "pdfinfo", pdfPath).Output()
	if err != nil {
		return 0, fmt.Errorf("pdfinfo: %w", err)
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		if after, ok := strings.CutPrefix(line, "Pages:"); ok {
			parts := strings.TrimSpace(after)
			n, err := strconv.Atoi(parts)
			if err != nil {
				return 0, fmt.Errorf("parse page count %q: %w", parts, err)
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("pages field not found in pdfinfo output")
}

// RenderPages renders all pages of a PDF to PNGs at the given DPI.
// Returns paths to the rendered PNG files sorted by page number.
func RenderPages(ctx context.Context, pdfPath string, dpi int, outDir string) ([]string, error) {
	logger := logutil.FromCtx(ctx)
	start := time.Now()

	prefix := filepath.Join(outDir, "page")
	cmd := exec.CommandContext(ctx, "pdftoppm",
		"-png",
		"-r", strconv.Itoa(dpi),
		pdfPath,
		prefix,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Glob for rendered PNGs and sort by name (page order).
	matches, err := filepath.Glob(prefix + "-*.png")
	if err != nil {
		return nil, fmt.Errorf("glob rendered pages: %w", err)
	}
	if len(matches) == 0 {
		// pdftoppm may use different naming for single-page PDFs.
		matches, err = filepath.Glob(prefix + "*.png")
		if err != nil {
			return nil, fmt.Errorf("glob rendered pages (fallback): %w", err)
		}
	}

	sort.Strings(matches)

	logger.Debug("pdftoppm complete",
		"path", pdfPath,
		"pages", len(matches),
		"dpi", dpi,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	if len(matches) == 0 {
		return nil, fmt.Errorf("pdftoppm produced no output files for %s", pdfPath)
	}

	return matches, nil
}

// ExtractPageText extracts raw text from a single page using pdftotext.
func ExtractPageText(ctx context.Context, pdfPath string, page int) (string, error) {
	logger := logutil.FromCtx(ctx)
	start := time.Now()

	pageStr := strconv.Itoa(page)
	out, err := exec.CommandContext(ctx, "pdftotext",
		"-f", pageStr,
		"-l", pageStr,
		pdfPath,
		"-", // output to stdout
	).Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext page %d: %w", page, err)
	}

	logger.Debug("pdftotext page complete",
		"path", pdfPath,
		"page", page,
		"chars", len(out),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	return string(out), nil
}

// ExtractAllText extracts raw text from all pages of a PDF.
func ExtractAllText(ctx context.Context, pdfPath string) (string, error) {
	out, err := exec.CommandContext(ctx, "pdftotext", pdfPath, "-").Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext: %w", err)
	}
	return string(out), nil
}
