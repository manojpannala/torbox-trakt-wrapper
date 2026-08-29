package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/spf13/cobra"
)

var (
	addCategoryFlag string
	seedFlag        int
)

var addCmd = &cobra.Command{
	Use:   "add <magnet|url>",
	Short: "Add a new download to TorBox cloud storage",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetConfig()
		if c.TorBox.APIKey == "" {
			return fmt.Errorf("torbox api_key is not configured")
		}

		link := strings.TrimSpace(args[0])
		if link == "" {
			return fmt.Errorf("download link cannot be empty")
		}

		client := torbox.NewClient(c.TorBox.APIKey)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		cat := addCategoryFlag
		if cat == "" {
			if strings.HasPrefix(link, "magnet:?") || strings.HasSuffix(link, ".torrent") {
				cat = "torrents"
			} else if strings.HasSuffix(link, ".nzb") {
				cat = "usenet"
			} else {
				cat = "webdl"
			}
		}

		switch strings.ToLower(cat) {
		case "torrents", "torrent":
			seedVal := 1
			if seedFlag > 0 {
				seedVal = seedFlag
			}
			resp, err := client.CreateTorrent(ctx, torbox.CreateTorrentRequest{
				Magnet: link,
				Seed:   seedVal,
			})
			if err != nil {
				return fmt.Errorf("adding torrent: %w", err)
			}
			fmt.Printf("✓ Torrent queued successfully! ID: %d (Hash: %s)\n", resp.TorrentID, resp.Hash)

		case "usenet":
			resp, err := client.CreateUsenet(ctx, torbox.CreateUsenetRequest{
				Link: link,
			})
			if err != nil {
				return fmt.Errorf("adding usenet download: %w", err)
			}
			fmt.Printf("✓ Usenet download queued successfully! ID: %d (Hash: %s)\n", resp.UsenetID, resp.Hash)

		case "webdl":
			resp, err := client.CreateWebDL(ctx, torbox.CreateWebDLRequest{
				Link: link,
			})
			if err != nil {
				return fmt.Errorf("adding web download: %w", err)
			}
			fmt.Printf("✓ Web download queued successfully! ID: %d (Hash: %s)\n", resp.WebDLID, resp.Hash)

		default:
			return fmt.Errorf("unknown category: %s (must be torrents, usenet, or webdl)", cat)
		}

		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addCategoryFlag, "category", "C", "", "Explicit category (torrents, usenet, webdl)")
	addCmd.Flags().IntVarP(&seedFlag, "seed", "s", 1, "Seeding mode for torrents (1: normal, 2: auto, 3: ignore)")
	rootCmd.AddCommand(addCmd)
}
