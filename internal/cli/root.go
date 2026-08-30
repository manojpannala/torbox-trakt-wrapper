package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

var (
	cfgFile string
	cfg     *config.Config
	cfgErr  error
)

var rootCmd = &cobra.Command{
	Use:   "tt-wrapper",
	Short: "TorBox and Trakt streaming wrapper and manager",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cfgErr
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		p := tea.NewProgram(tui.NewAppModel(ctx, cfg), tea.WithAltScreen())
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
	if cfgFile != "" {
		cfg, cfgErr = config.LoadFromFile(cfgFile)
	} else {
		cfg, cfgErr = config.Load()
	}
	if cfgErr != nil {
		cfg = nil
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
