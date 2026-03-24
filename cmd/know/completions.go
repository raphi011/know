package main

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/spf13/cobra"
)

type pathFilter int

const (
	pathFilterAll     pathFilter = iota // files + folders
	pathFilterFiles                     // files only
	pathFilterFolders                   // folders only
)

var noFileComp = cobra.ShellCompDirectiveNoFileComp

// noFileCompletions is a completion function that disables file completion.
func noFileCompletions(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return nil, noFileComp
}

// completeLabelNames returns a completion function that lists label names from the REST API.
func completeLabelNames(vaultFlag *string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		if vaultFlag == nil || *vaultFlag == "" {
			return nil, noFileComp
		}
		client := globalAPI.newClient()
		labels, err := client.ListLabels(context.Background(), *vaultFlag)
		if err != nil {
			cobra.CompDebugln(fmt.Sprintf("failed to list labels: %v", err), true)
			return nil, noFileComp
		}
		return labels, noFileComp
	}
}

// completeVaultNames returns a completion function that lists vault names from the REST API.
func completeVaultNames() func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		client := globalAPI.newClient()
		vaults, err := client.ListVaults(context.Background())
		if err != nil {
			cobra.CompDebugln(fmt.Sprintf("failed to list vaults: %v", err), true)
			return nil, noFileComp
		}
		names := make([]string, len(vaults))
		for i, v := range vaults {
			names[i] = v.Name
		}
		return names, noFileComp
	}
}

// completeVaultPaths returns a completion function that lists vault paths from the REST API.
// It parses the typed prefix to determine the parent directory, calls GET /api/ls,
// and filters results based on the pathFilter.
func completeVaultPaths(vaultFlag *string, filter pathFilter) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		directive := noFileComp | cobra.ShellCompDirectiveNoSpace

		if vaultFlag == nil || *vaultFlag == "" {
			return nil, noFileComp
		}

		// Parse prefix to determine parent directory and name filter
		if !strings.HasPrefix(toComplete, "/") {
			toComplete = "/" + toComplete
		}

		var parentDir, prefix string
		if strings.HasSuffix(toComplete, "/") {
			parentDir = toComplete
			prefix = ""
		} else {
			parentDir = path.Dir(toComplete) + "/"
			prefix = path.Base(toComplete)
		}

		client := globalAPI.newClient()
		entries, err := client.ListFiles(context.Background(), *vaultFlag, parentDir, false)
		if err != nil {
			cobra.CompDebugln(fmt.Sprintf("failed to list vault paths: %v", err), true)
			return nil, noFileComp
		}

		var completions []string
		for _, e := range entries {
			// Apply filter
			switch filter {
			case pathFilterFiles:
				if e.IsDir {
					continue
				}
			case pathFilterFolders:
				if !e.IsDir {
					continue
				}
			}

			// Filter by typed prefix
			if prefix != "" && !strings.HasPrefix(e.Name, prefix) {
				continue
			}

			p := e.Path
			if e.IsDir {
				p += "/"
			}
			completions = append(completions, p)
		}

		return completions, directive
	}
}
