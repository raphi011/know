package pathutil

import "strings"

// NormalizeFolderPath ensures the path has a leading and trailing slash.
func NormalizeFolderPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

// IsImmediateChild returns true if childPath is a direct child of folderPath.
// Both paths should use "/" separators. folderPath must end with "/".
func IsImmediateChild(folderPath, childPath string) bool {
	rel := strings.TrimPrefix(childPath, folderPath)
	return rel != childPath && rel != "" && !strings.Contains(rel, "/")
}

// IsImmediateChildFolder returns true if folderChildPath is a direct child
// folder of parentPath. Both paths should use "/" separators and end with "/".
func IsImmediateChildFolder(parentPath, folderChildPath string) bool {
	if !strings.HasPrefix(folderChildPath, parentPath) {
		return false
	}
	rel := strings.TrimPrefix(folderChildPath, parentPath)
	return rel != "" && !strings.Contains(strings.TrimSuffix(rel, "/"), "/")
}
