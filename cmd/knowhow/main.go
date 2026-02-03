// Package main provides the entry point for the knowhow CLI.
package main

import (
	"fmt"
	"os"

	"github.com/raphaelgruber/memcp-go/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
