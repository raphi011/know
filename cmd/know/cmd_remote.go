package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote know server connections",
}

var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured remotes",
	RunE:  runRemoteList,
}

var remoteAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add a remote know server",
	Long:  `Add a remote know server. Token can be passed via --token flag or KNOW_REMOTE_TOKEN env var. If neither is set, you will be prompted.`,
	Args:  cobra.ExactArgs(2),
	RunE:  runRemoteAdd,
}

var remoteRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a remote know server",
	Args:  cobra.ExactArgs(1),
	RunE:  runRemoteRemove,
}

var remoteAPIFlags *apiFlags
var remoteToken string

func init() {
	remoteAPIFlags = addAPIFlags(remoteCmd)
	remoteAddCmd.Flags().StringVar(&remoteToken, "token", os.Getenv("KNOW_REMOTE_TOKEN"), "API token for the remote server")
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
}

type remoteListResponse struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt"`
}

func runRemoteList(_ *cobra.Command, _ []string) error {
	client := remoteAPIFlags.newClient()

	var remotes []remoteListResponse
	if err := client.Get(context.Background(), "/api/v1/remotes", &remotes); err != nil {
		return fmt.Errorf("list remotes: %w", err)
	}

	if len(remotes) == 0 {
		fmt.Println("No remotes configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tCREATED")
	for _, r := range remotes {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.URL, r.CreatedAt.Format("2006-01-02"))
	}
	return w.Flush()
}

func runRemoteAdd(_ *cobra.Command, args []string) error {
	name := args[0]
	url := args[1]

	if remoteToken == "" {
		return fmt.Errorf("--token or KNOW_REMOTE_TOKEN required")
	}

	client := remoteAPIFlags.newClient()

	body := map[string]string{
		"name":  name,
		"url":   url,
		"token": remoteToken,
	}

	var result json.RawMessage
	if err := client.Post(context.Background(), "/api/v1/remotes", body, &result); err != nil {
		return fmt.Errorf("add remote: %w", err)
	}

	fmt.Printf("Remote %q added (%s)\n", name, url)
	return nil
}

func runRemoteRemove(_ *cobra.Command, args []string) error {
	name := args[0]
	client := remoteAPIFlags.newClient()

	if err := client.Delete(context.Background(), "/api/remotes/"+name); err != nil {
		return fmt.Errorf("remove remote: %w", err)
	}

	fmt.Printf("Remote %q removed\n", name)
	return nil
}
