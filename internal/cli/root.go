package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/tui"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/logging"
)

var (
	cfgFile  string
	verbose  bool
	cfg      *config.Config
	cfgErr   error
	logger   = slog.New(slog.DiscardHandler)
	closeLog = func() error { return nil }
)

var rootCmd = &cobra.Command{
	Use:   "tt-wrapper",
	Short: "TorBox and Trakt streaming wrapper and manager",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cfgErr != nil {
			return cfgErr
		}
		log, closer, err := logging.New(verbose, config.GetLogFile())
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		logger, closeLog = log, closer
		logger.Debug("starting", "version", config.Version, "command", cmd.Name())
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		return closeLog()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		// The alt screen is set on the view itself; see tui.altScreenView.
		p := tea.NewProgram(tui.NewAppModel(ctx, cfg, tui.WithLogger(logger)))
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "write a debug log to $XDG_STATE_HOME/torbox-trakt-wrapper/tt-wrapper.log")

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

func GetLogger() *slog.Logger {
	return logger
}
