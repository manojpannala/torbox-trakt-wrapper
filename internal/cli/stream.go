package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var streamCmd = &cobra.Command{
	Use:   "stream <query|id>",
	Short: "Search library and stream media directly with MPV",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetConfig()
		if c.TorBox.APIKey == "" {
			return fmt.Errorf("torbox api_key is not configured")
		}

		query := strings.Join(args, " ")
		tbClient := torbox.NewClient(c.TorBox.APIKey)

		var trClient *trakt.Client
		if c.Trakt.ClientID != "" {
			trClient = trakt.NewClient(
				c.Trakt.ClientID,
				c.Trakt.ClientSecret,
				trakt.WithTokens(trakt.TokenResponse{
					AccessToken:  c.Trakt.AccessToken,
					RefreshToken: c.Trakt.RefreshToken,
					CreatedAt:    c.Trakt.TokenCreatedAt,
					ExpiresIn:    c.Trakt.TokenExpiresIn,
				}),
				trakt.WithOnTokenRefreshed(func(tokens trakt.TokenResponse) {
					c.Trakt.AccessToken = tokens.AccessToken
					c.Trakt.RefreshToken = tokens.RefreshToken
					c.Trakt.TokenCreatedAt = tokens.CreatedAt
					c.Trakt.TokenExpiresIn = tokens.ExpiresIn
					_ = c.Save()
				}),
			)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		torrents, err := tbClient.GetTorrents(ctx, true)
		if err != nil {
			return fmt.Errorf("fetching torrents: %w", err)
		}

		var matchedTorrent *torbox.Torrent
		targetID, errID := strconv.Atoi(query)

		for i, t := range torrents {
			if errID == nil && t.ID == targetID {
				matchedTorrent = &torrents[i]
				break
			}
			if strings.Contains(strings.ToLower(t.Name), strings.ToLower(query)) {
				matchedTorrent = &torrents[i]
				break
			}
		}

		if matchedTorrent == nil {
			return fmt.Errorf("no media found matching query: %q", query)
		}

		parsed := matcher.ParseMedia(matchedTorrent.Name)
		var fileID int
		if len(matchedTorrent.Files) > 0 {
			bestFileIdx := 0
			var maxSizeBytes int64 = 0
			for idx, f := range matchedTorrent.Files {
				if f.Size > maxSizeBytes {
					maxSizeBytes = f.Size
					bestFileIdx = idx
				}
			}
			fileID = matchedTorrent.Files[bestFileIdx].ID
		}

		fmt.Printf("Resolving stream URL for: %s (ID: %d)...\n", parsed.DisplayTitle(), matchedTorrent.ID)
		streamLink, err := tbClient.RequestDownloadLink(ctx, matchedTorrent.ID, fileID, false)
		if err != nil {
			return fmt.Errorf("resolving stream URL: %w", err)
		}

		var scrobbler player.ScrobbleHandler
		if trClient != nil && c.Trakt.HasAuth() {
			scrobbler = player.NewTraktScrobbler(trClient)
		}

		mpv := player.NewMPVPlayer(
			player.WithExecutable(c.Player.Command),
			player.WithExtraArgs(c.Player.Args),
			player.WithIPCEnabled(c.Player.EnableIPC),
			player.WithScrobbler(scrobbler),
		)

		fmt.Printf("Launching %s in MPV...\n", parsed.DisplayTitle())

		streamMedia := player.MediaStream{
			URL:    streamLink,
			Title:  parsed.DisplayTitle(),
			Parsed: parsed,
		}

		session, err := mpv.Play(context.Background(), streamMedia)
		if err != nil {
			return fmt.Errorf("launching player: %w", err)
		}

		return session.Wait()
	},
}

func init() {
	rootCmd.AddCommand(streamCmd)
}
