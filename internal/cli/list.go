package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var (
	jsonOutput bool
	categoryFlag string
)

type ListItemOutput struct {
	ID            int     `json:"id"`
	Category      string  `json:"category"`
	Name          string  `json:"name"`
	CleanTitle    string  `json:"clean_title"`
	Size          int64   `json:"size"`
	FormattedSize string  `json:"formatted_size"`
	Status        string  `json:"status"`
	Progress      float64 `json:"progress"`
	TraktBadge    string  `json:"trakt_badge,omitempty"`
	TraktStatus   string  `json:"trakt_status,omitempty"`
}

var listCmd = &cobra.Command{
	Use:   "list [torrents|usenet|webdl]",
	Short: "List media items from TorBox cloud library",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := GetConfig()
		if c.TorBox.APIKey == "" {
			return fmt.Errorf("torbox api_key is not configured")
		}

		tbClient := torbox.NewClient(c.TorBox.APIKey)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		var matcherEngine *matcher.Matcher
		if c.Trakt.HasAuth() {
			trClient := trakt.NewClient(
				c.Trakt.ClientID,
				c.Trakt.ClientSecret,
				trakt.WithTokens(trakt.TokenResponse{
					AccessToken:  c.Trakt.AccessToken,
					RefreshToken: c.Trakt.RefreshToken,
					CreatedAt:    c.Trakt.TokenCreatedAt,
					ExpiresIn:    c.Trakt.TokenExpiresIn,
				}),
			)
			movies, _ := trClient.GetWatchedMovies(ctx)
			shows, _ := trClient.GetWatchedShows(ctx)
			playback, _ := trClient.GetPlayback(ctx)
			matcherEngine = matcher.NewMatcher(movies, shows, playback)
		} else {
			matcherEngine = matcher.NewMatcher(nil, nil, nil)
		}

		cat := "torrents"
		if len(args) > 0 {
			cat = strings.ToLower(args[0])
		} else if categoryFlag != "" {
			cat = strings.ToLower(categoryFlag)
		}

		var items []ListItemOutput

		switch cat {
		case "torrents", "torrent":
			torrents, err := tbClient.GetTorrents(ctx, true)
			if err != nil {
				return fmt.Errorf("fetching torrents: %w", err)
			}
			for _, t := range torrents {
				parsed := matcher.ParseMedia(t.Name)
				matchRes := matcherEngine.MatchParsed(parsed)
				badge := matchRes.Badge
				statusStr := watchStatusString(matchRes.Status)

				if len(t.Files) > 1 {
					fileResults := matcherEngine.MatchTorrentFiles(t.Files)
					folderStatus := matcher.AggregateFolderStatus(fileResults)
					badge = folderStatus.Badge
					statusStr = watchStatusString(folderStatus.Status)
				}

				items = append(items, ListItemOutput{
					ID:            t.ID,
					Category:      "torrents",
					Name:          t.Name,
					CleanTitle:    parsed.DisplayTitle(),
					Size:          t.Size,
					FormattedSize: formatBytes(t.Size),
					Status:        t.DownloadState,
					Progress:      t.Progress * 100,
					TraktBadge:    badge,
					TraktStatus:   statusStr,
				})
			}

		case "usenet":
			usenet, err := tbClient.GetUsenetList(ctx, true)
			if err != nil {
				return fmt.Errorf("fetching usenet: %w", err)
			}
			for _, u := range usenet {
				parsed := matcher.ParseMedia(u.Name)
				matchRes := matcherEngine.MatchParsed(parsed)
				items = append(items, ListItemOutput{
					ID:            u.ID,
					Category:      "usenet",
					Name:          u.Name,
					CleanTitle:    parsed.DisplayTitle(),
					Size:          u.Size,
					FormattedSize: formatBytes(u.Size),
					Status:        u.DownloadState,
					Progress:      u.Progress * 100,
					TraktBadge:    matchRes.Badge,
					TraktStatus:   watchStatusString(matchRes.Status),
				})
			}

		case "webdl":
			webdl, err := tbClient.GetWebDLList(ctx, true)
			if err != nil {
				return fmt.Errorf("fetching webdl: %w", err)
			}
			for _, w := range webdl {
				parsed := matcher.ParseMedia(w.Name)
				matchRes := matcherEngine.MatchParsed(parsed)
				items = append(items, ListItemOutput{
					ID:            w.ID,
					Category:      "webdl",
					Name:          w.Name,
					CleanTitle:    parsed.DisplayTitle(),
					Size:          w.Size,
					FormattedSize: formatBytes(w.Size),
					Status:        w.DownloadState,
					Progress:      w.Progress * 100,
					TraktBadge:    matchRes.Badge,
					TraktStatus:   watchStatusString(matchRes.Status),
				})
			}

		default:
			return fmt.Errorf("unknown category: %s (must be torrents, usenet, or webdl)", cat)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(items)
		}

		if len(items) == 0 {
			fmt.Println("No items found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tTRAKT\tTITLE\tSIZE\tSTATUS")
		for _, item := range items {
			badge := item.TraktBadge
			if badge == "" {
				badge = "-"
			}
			status := item.Status
			if status == "downloading" {
				status = fmt.Sprintf("%.0f%%", item.Progress)
			}
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", item.ID, badge, item.CleanTitle, item.FormattedSize, status)
		}
		return w.Flush()
	},
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func watchStatusString(s matcher.WatchStatus) string {
	switch s {
	case matcher.StatusWatched:
		return "watched"
	case matcher.StatusInProgress:
		return "in_progress"
	default:
		return "unwatched"
	}
}

func init() {
	listCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output results in JSON format")
	listCmd.Flags().StringVarP(&categoryFlag, "category", "C", "", "Category filter (torrents, usenet, webdl)")
	rootCmd.AddCommand(listCmd)
}
