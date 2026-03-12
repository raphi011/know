package tools

import "github.com/raphi011/knowhow/internal/pathutil"

// Slugify converts a title to a URL-friendly slug.
// Delegates to pathutil.Slugify to avoid import cycles.
func Slugify(title string) string {
	return pathutil.Slugify(title)
}
