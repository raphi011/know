package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/auth"
	"github.com/spf13/cobra"
)

// AuthConfig is stored in ~/.config/know/auth.json.
type AuthConfig struct {
	APIURL string `json:"api_url"`
	Token  string `json:"token"`
}

func authConfigPath() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "know", "auth.json")
}

func loadAuthConfig() (*AuthConfig, error) {
	data, err := os.ReadFile(authConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cfg AuthConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveAuthConfig(cfg *AuthConfig) error {
	dir := filepath.Dir(authConfigPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(authConfigPath(), data, 0600)
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication management",
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to a Know server",
	RunE: func(cmd *cobra.Command, args []string) error {
		apiURL, _ := cmd.Flags().GetString("api-url")

		fmt.Println("How would you like to authenticate?")
		fmt.Println("  1. Login with a web browser (OIDC)")
		fmt.Println("  2. Paste an API token")
		fmt.Print("\nChoice [1/2]: ")

		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "1", "":
			return loginWithBrowser(apiURL)
		case "2":
			return loginWithToken(apiURL)
		default:
			return fmt.Errorf("invalid choice: %s", choice)
		}
	},
}

func loginWithBrowser(apiURL string) error {
	ctx := context.Background()
	client := apiclient.New(apiURL, "")

	resp, err := client.DeviceFlowStart(ctx)
	if err != nil {
		return fmt.Errorf("start device flow: %w", err)
	}

	fmt.Printf("\nFirst copy your one-time code: %s\n", resp.UserCode)
	fmt.Print("Press Enter to open the browser...")
	fmt.Scanln()

	openBrowser(resp.VerificationURI)

	fmt.Println("\nWaiting for authentication...")

	ticker := time.NewTicker(time.Duration(resp.Interval) * time.Second)
	defer ticker.Stop()
	timeout := time.After(time.Duration(resp.ExpiresIn) * time.Second)

	for {
		select {
		case <-ticker.C:
			token, err := client.DeviceFlowPoll(ctx, resp.DeviceCode)
			if err != nil {
				// authorization_pending or other transient error — keep polling
				continue
			}
			if token != "" {
				if err := saveAuthConfig(&AuthConfig{APIURL: apiURL, Token: token}); err != nil {
					return fmt.Errorf("save auth config: %w", err)
				}
				fmt.Println("\nLogin successful! Token saved.")
				return nil
			}
		case <-timeout:
			return fmt.Errorf("login timed out — please try again")
		}
	}
}

func loginWithToken(apiURL string) error {
	fmt.Print("Paste your API token: ")
	var token string
	fmt.Scanln(&token)

	if err := auth.ValidateTokenFormat(token); err != nil {
		return fmt.Errorf("invalid token format: %w", err)
	}

	// Verify the token works by listing vaults.
	client := apiclient.New(apiURL, token)
	if _, err := client.ListVaults(context.Background()); err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	if err := saveAuthConfig(&AuthConfig{APIURL: apiURL, Token: token}); err != nil {
		return fmt.Errorf("save auth config: %w", err)
	}
	fmt.Println("Token saved successfully!")
	return nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start() // best-effort; user can open URL manually
	}
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE: func(cmd *cobra.Command, args []string) error {
		if token := os.Getenv("KNOW_TOKEN"); token != "" {
			fmt.Println("Token source: KNOW_TOKEN environment variable")
			return nil
		}

		cfg, err := loadAuthConfig()
		if err != nil {
			return fmt.Errorf("load auth config: %w", err)
		}
		if cfg == nil {
			fmt.Println("Not logged in.")
			return nil
		}

		fmt.Printf("Logged in to: %s\n", cfg.APIURL)
		if len(cfg.Token) > 10 {
			fmt.Printf("Token: %s...%s\n", cfg.Token[:6], cfg.Token[len(cfg.Token)-4:])
		} else {
			fmt.Println("Token: (stored)")
		}
		return nil
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored authentication token",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := authConfigPath()
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				fmt.Println("Not logged in.")
				return nil
			}
			return fmt.Errorf("remove auth config: %w", err)
		}
		fmt.Println("Logged out successfully.")
		return nil
	},
}

func init() {
	authLoginCmd.Flags().String("api-url", envOrDefault("KNOW_SERVER_URL", "http://localhost:4001"), "Server URL")
	if err := authLoginCmd.RegisterFlagCompletionFunc("api-url", noFileCompletions); err != nil {
		panic(fmt.Sprintf("register api-url completion: %v", err))
	}
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
}
