package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	configPath := flag.String("config", "", "path to config file (default: ~/.config/knowhow-mcp/config.toml)")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	slog.Info("loaded config",
		"instances", len(cfg.Instances),
		"port", cfg.Port,
	)

	tools := newMCPTools(cfg)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "knowhow-mcp",
		Version: "0.1.0",
	}, nil)

	tools.register(server)

	handler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		nil,
	)

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("starting MCP server", "addr", addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
