package web

import (
	"context"
	"fmt"
	"path"

	"github.com/raphi011/knowhow/internal/models"
	"github.com/raphi011/knowhow/internal/web/templates/components"
)

// buildSidebar constructs the sidebar data by listing all folders in a vault
// and building them into a tree structure.
func (h *Handler) buildSidebar(ctx context.Context, vaultID string) (components.SidebarData, error) {
	v, err := h.vaultSvc.Get(ctx, vaultID)
	if err != nil {
		return components.SidebarData{VaultID: vaultID}, fmt.Errorf("get vault: %w", err)
	}
	vaultName := vaultID
	if v != nil {
		vaultName = v.Name
	}

	folders, err := h.vaultSvc.ListFolders(ctx, vaultID, nil)
	if err != nil {
		return components.SidebarData{VaultID: vaultID, VaultName: vaultName}, fmt.Errorf("list folders: %w", err)
	}

	tree := buildFolderTree(folders)

	return components.SidebarData{
		VaultID:   vaultID,
		VaultName: vaultName,
		Folders:   tree,
	}, nil
}

// buildFolderTree constructs a nested tree from a flat list of folders.
func buildFolderTree(folders []models.Folder) []components.FolderNode {
	// Group by parent path
	childrenOf := make(map[string][]components.FolderNode)
	for _, f := range folders {
		parent := path.Dir(f.Path)
		if parent == "." {
			parent = "/"
		}
		childrenOf[parent] = append(childrenOf[parent], components.FolderNode{
			Name: f.Name,
			Path: f.Path,
		})
	}

	// Recursively build tree from root
	var build func(parentPath string) []components.FolderNode
	build = func(parentPath string) []components.FolderNode {
		children := childrenOf[parentPath]
		for i := range children {
			children[i].Children = build(children[i].Path)
		}
		return children
	}

	return build("/")
}
