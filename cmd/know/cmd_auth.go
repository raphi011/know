package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/browser"
	"github.com/spf13/cobra"
)

// AuthConfig is stored in ~/.config/know/auth.json.
type AuthConfig struct {
	APIURL string `json:"api_url"`
	Token  string `json:"token"`
}

func authConfigPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(configDir, "know", "auth.json"), nil
}

func loadAuthConfig() (*AuthConfig, error) {
	path, err := authConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
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
	path, err := authConfigPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
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

		// Try device flow — succeeds when OIDC is enabled, 404 when not.
		client := apiclient.New(apiURL, "")
		resp, err := client.DeviceFlowStart(context.Background())
		if err != nil {
			// OIDC not available — go straight to token login.
			return loginWithToken(apiURL)
		}

		// OIDC available — let the user choose.
		fmt.Println("How would you like to authenticate?")
		fmt.Println("  1. Login with a web browser (OIDC)")
		fmt.Println("  2. Paste an API token")
		fmt.Print("\nChoice [1/2]: ")

		var choice string
		fmt.Scanln(&choice)

		switch choice {
		case "1", "":
			return loginWithDeviceFlow(apiURL, resp)
		case "2":
			return loginWithToken(apiURL)
		default:
			return fmt.Errorf("invalid choice: %s", choice)
		}
	},
}

func loginWithDeviceFlow(apiURL string, resp *apiclient.DeviceFlowStartResponse) error {
	ctx := context.Background()
	client := apiclient.New(apiURL, "")

	fmt.Printf("\nFirst copy your one-time code: %s\n", resp.UserCode)
	fmt.Print("Press Enter to open the browser...")
	fmt.Scanln()

	if err := browser.Open(resp.VerificationURI); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser: %v\nPlease open this URL manually: %s\n", err, resp.VerificationURI)
	}

	fmt.Println("\nWaiting for authentication...")

	ticker := time.NewTicker(time.Duration(resp.Interval) * time.Second)
	defer ticker.Stop()
	timeout := time.After(time.Duration(resp.ExpiresIn) * time.Second)

	for {
		select {
		case <-ticker.C:
			token, err := client.DeviceFlowPoll(ctx, resp.DeviceCode)
			if err != nil {
				var httpErr *apiclient.HTTPError
				if errors.As(err, &httpErr) {
					switch httpErr.StatusCode {
					case 428: // authorization_pending — keep polling
						continue
					case 410: // expired
						return fmt.Errorf("device code expired — please try again")
					case 404: // not found
						return fmt.Errorf("device code not found — please try again")
					}
				}
				return fmt.Errorf("poll device flow: %w", err)
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
		path, err := authConfigPath()
		if err != nil {
			return fmt.Errorf("auth config path: %w", err)
		}
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
