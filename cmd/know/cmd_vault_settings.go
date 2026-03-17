package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/raphi011/know/internal/models"
	"github.com/spf13/cobra"
)

var (
	vaultSettingsAPI     *apiFlags
	vaultSettingsVaultID *string
	vaultSettingsJSON    bool
	vaultSettingsSet     []string
)

var vaultSettingsCmd = &cobra.Command{
	Use:   "settings [vault-name]",
	Short: "View or update vault settings",
	Long: `View or update per-vault settings such as folder paths and memory parameters.

The vault name can be passed as a positional argument or via --vault flag.

To update settings, use --set key=value (repeatable):
  know vault settings --set daily_note_path=/notes --set template_path=/tpl

Valid keys:
  memory_path              folder for memories (default: /memories)
  memory_merge_threshold   similarity threshold for memory consolidation (default: 0.95)
  memory_archive_threshold score below which memories are archived (default: 0.2)
  memory_decay_half_life   days until memory score halves (default: 30)
  template_path            folder for templates (default: /templates)
  daily_note_path          folder for daily notes (default: /daily)
  transcript_template      template path for LLM transcript summarization (empty = disabled, set to "-" to clear)
  rrf_k                    RRF K parameter for hybrid search (default: 60)
  hnsw_ef                  HNSW EF parameter for vector search (default: 40)
  default_search_limit     default number of search results (default: 20)
  max_search_limit         maximum search result limit (default: 100)
  version_coalesce_minutes minutes between version snapshots (default: 10)
  version_retention_count  max versions per file (default: 50)

Environment variables:
  KNOW_VAULT    vault name (alternative to --vault flag)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runVaultSettings,
}

func init() {
	vaultSettingsAPI = addAPIFlags(vaultSettingsCmd)
	vaultSettingsVaultID = addVaultFlag(vaultSettingsCmd, vaultSettingsAPI)
	vaultSettingsCmd.Flags().BoolVar(&vaultSettingsJSON, "json", false, "output as JSON")
	vaultSettingsCmd.Flags().StringArrayVar(&vaultSettingsSet, "set", nil, "set a setting (key=value, repeatable)")
	vaultSettingsCmd.ValidArgsFunction = completeVaultNames(vaultSettingsAPI)

	vaultCmd.AddCommand(vaultSettingsCmd)
}

func runVaultSettings(_ *cobra.Command, args []string) error {
	vaultName := *vaultSettingsVaultID
	if len(args) > 0 {
		vaultName = args[0]
	}

	client := vaultSettingsAPI.newClient()
	ctx := context.Background()

	if len(vaultSettingsSet) > 0 {
		patch, err := parseSettingsPatch(vaultSettingsSet)
		if err != nil {
			return err
		}
		settings, err := client.UpdateVaultSettings(ctx, vaultName, patch)
		if err != nil {
			return fmt.Errorf("update settings: %w", err)
		}
		return printSettings(settings)
	}

	settings, err := client.GetVaultSettings(ctx, vaultName)
	if err != nil {
		return fmt.Errorf("get settings: %w", err)
	}
	return printSettings(settings)
}

func parseSettingsPatch(pairs []string) (models.VaultSettings, error) {
	var s models.VaultSettings
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return s, fmt.Errorf("invalid --set format %q, expected key=value", pair)
		}
		switch key {
		case "memory_path":
			s.MemoryPath = value
		case "template_path":
			s.TemplatePath = value
		case "daily_note_path":
			s.DailyNotePath = value
		case "transcript_template":
			s.TranscriptTemplate = value
		case "memory_merge_threshold":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.MemoryMergeThreshold = v
		case "memory_archive_threshold":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.MemoryArchiveThreshold = v
		case "memory_decay_half_life":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.MemoryDecayHalfLife = v
		case "rrf_k":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.RRFK = v
		case "hnsw_ef":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.HNSWEF = v
		case "default_search_limit":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.DefaultSearchLimit = v
		case "max_search_limit":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.MaxSearchLimit = v
		case "version_coalesce_minutes":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.VersionCoalesceMinutes = v
		case "version_retention_count":
			v, err := strconv.Atoi(value)
			if err != nil {
				return s, fmt.Errorf("invalid value for %s: %w", key, err)
			}
			s.VersionRetentionCount = v
		default:
			return s, fmt.Errorf("unknown setting %q", key)
		}
	}
	return s, nil
}

func printSettings(s *models.VaultSettings) error {
	if vaultSettingsJSON {
		out, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("%-30s %s\n", "memory_path:", s.MemoryPath)
	fmt.Printf("%-30s %.2f\n", "memory_merge_threshold:", s.MemoryMergeThreshold)
	fmt.Printf("%-30s %.2f\n", "memory_archive_threshold:", s.MemoryArchiveThreshold)
	fmt.Printf("%-30s %d\n", "memory_decay_half_life:", s.MemoryDecayHalfLife)
	fmt.Printf("%-30s %s\n", "template_path:", s.TemplatePath)
	fmt.Printf("%-30s %s\n", "daily_note_path:", s.DailyNotePath)
	fmt.Printf("%-30s %s\n", "transcript_template:", s.TranscriptTemplate)
	fmt.Printf("%-30s %d\n", "rrf_k:", s.RRFK)
	fmt.Printf("%-30s %d\n", "hnsw_ef:", s.HNSWEF)
	fmt.Printf("%-30s %d\n", "default_search_limit:", s.DefaultSearchLimit)
	fmt.Printf("%-30s %d\n", "max_search_limit:", s.MaxSearchLimit)
	fmt.Printf("%-30s %d\n", "version_coalesce_minutes:", s.VersionCoalesceMinutes)
	fmt.Printf("%-30s %d\n", "version_retention_count:", s.VersionRetentionCount)
	return nil
}
