package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/raphi011/know/internal/apiclient"
	"github.com/raphi011/know/internal/auth"
	"github.com/raphi011/know/internal/browser"
	"github.com/raphi011/know/internal/keychain"
	"github.com/spf13/cobra"
)

// saveCredentials stores the token and API URL in the system keychain.
func saveCredentials(apiURL, token string) error {
	if err := keychain.SetToken(token); err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	if err := keychain.SetAPIURL(apiURL); err != nil {
		return fmt.Errorf("save api url: %w", err)
	}
	return nil
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

		// Try device flow to discover if OIDC is available.
		// 200 = OIDC enabled, 404 = not enabled, other errors = warn and fall back.
		client := apiclient.New(apiURL, "")
		resp, err := client.DeviceFlowStart(context.Background())
		if err != nil {
			var httpErr *apiclient.HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
				fmt.Fprintf(os.Stderr, "Warning: could not start device flow: %v\n", err)
				fmt.Fprintln(os.Stderr, "Falling back to token-based login.")
			}
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
				if err := saveCredentials(apiURL, token); err != nil {
					return fmt.Errorf("save credentials: %w", err)
				}
				fmt.Println("\nLogin successful! Token saved to system keychain.")
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

	if err := saveCredentials(apiURL, token); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Println("Token saved to system keychain.")
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

		token, err := keychain.GetToken()
		if err != nil {
			if keychain.IsNotFound(err) {
				fmt.Println("Not logged in.")
				return nil
			}
			return fmt.Errorf("read keychain: %w", err)
		}

		apiURL, _ := keychain.GetAPIURL()

		fmt.Println("Token source: system keychain")
		if apiURL != "" {
			fmt.Printf("Server: %s\n", apiURL)
		}
		if len(token) > 10 {
			fmt.Printf("Token: %s...%s\n", token[:6], token[len(token)-4:])
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
		tokenErr := keychain.DeleteToken()
		urlErr := keychain.DeleteAPIURL()

		if tokenErr != nil && keychain.IsNotFound(tokenErr) && (urlErr == nil || keychain.IsNotFound(urlErr)) {
			fmt.Println("Not logged in.")
			return nil
		}
		if tokenErr != nil && !keychain.IsNotFound(tokenErr) {
			return fmt.Errorf("remove token: %w", tokenErr)
		}
		if urlErr != nil && !keychain.IsNotFound(urlErr) {
			return fmt.Errorf("remove api url: %w", urlErr)
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
