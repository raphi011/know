package document

import (
	"context"
	"strings"
	"time"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/models"
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

// isTemplatePath checks if a document path falls under the vault's template folder.
func (s *Service) isTemplatePath(ctx context.Context, vaultID, docPath string) bool {
	vault, err := s.db.GetVault(ctx, vaultID)
	if err != nil {
		logutil.FromCtx(ctx).Warn("failed to load vault for template check", "vault_id", vaultID, "error", err)
		return false
	}
	tplPath := vault.TemplatePath()
	if !strings.HasSuffix(tplPath, "/") {
		tplPath += "/"
	}
	return strings.HasPrefix(docPath, tplPath)
}

// ListTemplates returns document metadata for all templates in a vault.
func (s *Service) ListTemplates(ctx context.Context, vaultID string) ([]models.DocumentMeta, error) {
	vault, err := s.db.GetVault(ctx, vaultID)
	if err != nil {
		return nil, err
	}
	tplPath := vault.TemplatePath()
	return s.db.ListDocumentMetas(ctx, db.ListDocumentsFilter{
		VaultID: vaultID,
		Folder:  &tplPath,
	})
}
