package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth [trakt|torbox]",
	Short: "Authenticate with TorBox or Trakt.tv",
	Long:  "Authenticate with TorBox by setting an API token, or pair Trakt.tv using OAuth Device Flow.",
}

var authTraktCmd = &cobra.Command{
	Use:   "trakt",
	Short: "Pair with Trakt.tv using OAuth Device Flow",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetConfig()
		if c.Trakt.ClientID == "" {
			return fmt.Errorf("trakt client_id is not configured in config.toml")
		}

		client := trakt.NewClient(c.Trakt.ClientID, c.Trakt.ClientSecret)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := client.GenerateDeviceCode(ctx)
		if err != nil {
			return fmt.Errorf("failed to generate device code: %w", err)
		}

		fmt.Println("==================================================")
		fmt.Println(" Trakt.tv Device Pairing")
		fmt.Println("==================================================")
		fmt.Printf("1. Open URL in browser: %s\n", resp.VerificationURL)
		fmt.Printf("2. Enter pairing code:   %s\n", resp.UserCode)
		fmt.Println("==================================================")

		if clipErr := clipboard.WriteAll(resp.UserCode); clipErr == nil {
			fmt.Println("(Copied code to clipboard)")
		}

		fmt.Println("Waiting for authorization on trakt.tv...")

		pollCtx, pollCancel := context.WithTimeout(context.Background(), time.Duration(resp.ExpiresIn)*time.Second)
		defer pollCancel()

		tokens, err := client.PollDeviceToken(pollCtx, resp.DeviceCode, resp.Interval)
		if err != nil {
			return fmt.Errorf("device pairing failed: %w", err)
		}

		c.Trakt.AccessToken = tokens.AccessToken
		c.Trakt.RefreshToken = tokens.RefreshToken
		c.Trakt.TokenCreatedAt = tokens.CreatedAt
		c.Trakt.TokenExpiresIn = tokens.ExpiresIn

		if err := saveActiveConfig(c); err != nil {
			return fmt.Errorf("failed to save tokens to config: %w", err)
		}

		fmt.Println("✓ Successfully authenticated with Trakt.tv!")
		return nil
	},
}

var authTorBoxCmd = &cobra.Command{
	Use:   "torbox [api_key]",
	Short: "Set TorBox API key",
	RunE: func(cmd *cobra.Command, args []string) error {
		var apiKey string
		if len(args) > 0 {
			apiKey = strings.TrimSpace(args[0])
		} else {
			fmt.Print("Enter TorBox API Key: ")
			reader := bufio.NewReader(os.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			apiKey = strings.TrimSpace(line)
		}

		if apiKey == "" {
			return fmt.Errorf("api key cannot be empty")
		}

		c := GetConfig()
		c.TorBox.APIKey = apiKey
		if err := saveActiveConfig(c); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("✓ TorBox API key saved successfully.")
		return nil
	},
}

func saveActiveConfig(c *config.Config) error {
	path := cfgFile
	if path == "" {
		path = config.GetConfigFile()
	}
	return c.SaveToFile(path)
}

func init() {
	authCmd.AddCommand(authTraktCmd)
	authCmd.AddCommand(authTorBoxCmd)
	rootCmd.AddCommand(authCmd)
}
