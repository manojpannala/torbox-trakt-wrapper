package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage and inspect torbox-trakt-wrapper configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = config.GetConfigFile()
		}

		fmt.Printf("Config File: %s\n", path)
		fmt.Printf("Status:      %s\n", GetConfig().String())
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print configuration file path",
	Run: func(cmd *cobra.Command, args []string) {
		if cfgFile != "" {
			fmt.Println(cfgFile)
		} else {
			fmt.Println(config.GetConfigFile())
		}
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgFile
		if path == "" {
			path = config.GetConfigFile()
		}

		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config file already exists at %s", path)
		}

		c := config.DefaultConfig()
		if err := c.SaveToFile(path); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}

		fmt.Printf("Initialized default configuration at %s\n", path)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configInitCmd)
}
