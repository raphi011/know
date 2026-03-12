package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var dbWipeCmd = &cobra.Command{
	Use:   "wipe",
	Short: "Wipe all data from the database",
	RunE:  runDBWipe,
}

func runDBWipe(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbClient, err := connectDB(ctx)
	if err != nil {
		return err
	}
	defer dbClient.Close(ctx) //nolint:errcheck // process exits immediately; close failure is benign

	if err := dbClient.WipeData(ctx); err != nil {
		return fmt.Errorf("wipe data: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Wiped all data from the database\n")
	return nil
}
