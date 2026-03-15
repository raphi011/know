package file

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ApplyTemplateVars replaces built-in template placeholders (e.g. {{date}}) in content.
func ApplyTemplateVars(content string, vars map[string]string) string {
	replacements := make([]string, 0, len(vars)*2)
	for k, v := range vars {
		replacements = append(replacements, "{{"+k+"}}", v)
	}
	return strings.NewReplacer(replacements...).Replace(content)
}

// DefaultTemplateVars returns the standard built-in variables for template substitution.
func DefaultTemplateVars(t time.Time, title, vaultName string) map[string]string {
	return map[string]string{
		"date":     t.Format("2006-01-02"),
		"datetime": t.Format("2006-01-02 15:04"),
		"title":    title,
		"vault":    vaultName,
	}
}

// isTemplatePath checks if a file path falls under the vault's template folder.
func (s *Service) isTemplatePath(ctx context.Context, vaultID, filePath string) (bool, error) {
	vault, err := s.db.GetVault(ctx, vaultID)
	if err != nil {
		return false, fmt.Errorf("load vault for template check: %w", err)
	}
	return IsTemplatePath(vault.TemplatePath(), filePath), nil
}

// IsTemplatePath returns true if filePath falls under the given template folder path.
func IsTemplatePath(templateFolder, filePath string) bool {
	if !strings.HasSuffix(templateFolder, "/") {
		templateFolder += "/"
	}
	return strings.HasPrefix(filePath, templateFolder)
}
