package cli

import (
	"fmt"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("tt-wrapper %s (commit: %s, built: %s)\n", config.Version, config.Commit, config.Date)
	},
}
