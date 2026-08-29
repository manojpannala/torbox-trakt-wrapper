package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "tt-wrapper",
	Short: "TorBox and Trakt streaming wrapper and manager",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := tea.NewProgram(tui.NewAppModel(cfg), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $XDG_CONFIG_HOME/torbox-trakt-wrapper/config.toml)")

	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	var err error
	if cfgFile != "" {
		cfg, err = config.LoadFromFile(cfgFile)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		cfg = config.DefaultConfig()
	}
}

func GetConfig() *config.Config {
	if cfg == nil {
		initConfig()
	}
	return cfg
}

func GetRootCommand() *cobra.Command {
	return rootCmd
}
