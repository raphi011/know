package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/raphaelgruber/memcp-go/internal/config"
)

// DetectContext determines the project context for scoping entities.
// Priority: explicit config > git origin > cwd basename.
func DetectContext(cfg *config.Config) *string {
	// 1. Check explicit default context from config
	if cfg.DefaultContext != "" {
		return &cfg.DefaultContext
	}

	// 2. Check if context detection from CWD is enabled
	if !cfg.ContextFromCWD {
		return nil
	}

	// 3. Try git remote origin
	if origin := getGitOriginName(); origin != "" {
		return &origin
	}

	// 4. Fall back to CWD basename
	if cwd, err := os.Getwd(); err == nil {
		base := filepath.Base(cwd)
		return &base
	}

	return nil
}

// getGitOriginName extracts repo name from git remote origin URL.
// Handles: git@github.com:owner/repo.git, https://github.com/owner/repo.git
func getGitOriginName() string {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseRepoName(strings.TrimSpace(string(output)))
}

// parseRepoName extracts repo name from git URL.
func parseRepoName(url string) string {
	if url == "" {
		return ""
	}

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// SSH format: git@github.com:owner/repo
	if strings.HasPrefix(url, "git@") {
		parts := strings.Split(url, ":")
		if len(parts) == 2 {
			pathParts := strings.Split(parts[1], "/")
			if len(pathParts) > 0 {
				return pathParts[len(pathParts)-1]
			}
		}
	}

	// HTTPS format: https://github.com/owner/repo
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		parts := strings.Split(url, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	return ""
}

// extractID removes "entity:" prefix if present.
func extractID(id string) string {
	return strings.TrimPrefix(id, "entity:")
}
