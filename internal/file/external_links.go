package file

import (
	"context"
	"fmt"
	"net/url"

	"github.com/raphi011/know/internal/db"
	"github.com/raphi011/know/internal/logutil"
	"github.com/raphi011/know/internal/parser"
)

// processExternalLinks extracts external URLs from parsed markdown and stores
// them in the external_link table. Follows the same delete-then-insert pattern
// as processWikiLinks.
func (s *Service) processExternalLinks(ctx context.Context, fileID, vaultID string, links []parser.ExtractedLink) error {
	if err := s.db.DeleteExternalLinksByFile(ctx, fileID); err != nil {
		return fmt.Errorf("delete old: %w", err)
	}

	if len(links) == 0 {
		return nil
	}

	logger := logutil.FromCtx(ctx)

	inputs := make([]db.ExternalLinkInput, 0, len(links))
	for _, l := range links {
		u, err := url.Parse(l.URL)
		if err != nil {
			logger.Warn("skipping unparseable URL", "url", l.URL, "error", err)
			continue
		}
		if u.Hostname() == "" {
			logger.Debug("skipping URL with empty hostname", "url", l.URL)
			continue
		}

		input := db.ExternalLinkInput{
			Hostname: u.Hostname(),
			URLPath:  u.Path,
			FullURL:  l.URL,
		}
		if l.LinkText != "" {
			input.LinkText = &l.LinkText
		}
		inputs = append(inputs, input)
	}

	if err := s.db.CreateExternalLinks(ctx, fileID, vaultID, inputs); err != nil {
		return fmt.Errorf("create: %w", err)
	}
	return nil
}
