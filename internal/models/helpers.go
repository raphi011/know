package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strings"

	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// RecordIDString safely extracts the string ID from a SurrealDB RecordID.
func RecordIDString(id surrealmodels.RecordID) (string, error) {
	s, ok := id.ID.(string)
	if !ok {
		return "", fmt.Errorf("unexpected ID type: %T (expected string)", id.ID)
	}
	return s, nil
}

// MustRecordIDString extracts the string ID, panicking if not a string.
func MustRecordIDString(id surrealmodels.RecordID) string {
	s, err := RecordIDString(id)
	if err != nil {
		panic(err)
	}
	return s
}

// ContentHash computes SHA256 hash of content for dedup.
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// NormalizePath ensures path starts with / and has no trailing slash.
func NormalizePath(p string) string {
	p = path.Clean(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

// BareID strips a "table:" prefix from a record ID string so it can be
// used safely with type::record("table", id). Accepts both "default"
// and "vault:default" — returns "default" in both cases.
func BareID(table, id string) string {
	return strings.TrimPrefix(id, table+":")
}

// ParentFolder returns the parent folder path for a document path.
func ParentFolder(docPath string) string {
	dir := path.Dir(docPath)
	if dir == "." {
		return "/"
	}
	return dir
}
