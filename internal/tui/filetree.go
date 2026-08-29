package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
)

type FileTreeItem struct {
	ID            int
	Name          string
	CleanTitle    string
	Size          int64
	FormattedSize string
	Badge         string
	Status        matcher.WatchStatus
	Progress      float64
	Parsed        matcher.ParsedMedia
}

type FileTreeModel struct {
	ParentItem *LibraryItem
	Items      []FileTreeItem
	Cursor     int
	TopIndex   int
	Height     int
	Width      int
}

func NewFileTreeModel(parent *LibraryItem, matcherEngine *matcher.Matcher) FileTreeModel {
	ft := FileTreeModel{
		ParentItem: parent,
		Cursor:     0,
		TopIndex:   0,
	}

	if parent == nil {
		return ft
	}

	if len(parent.TorrentFiles) > 0 {
		for _, f := range parent.TorrentFiles {
			parsed := matcher.ParseMedia(f.Name)
			var badge string
			var status matcher.WatchStatus
			var progress float64

			if matcherEngine != nil {
				res := matcherEngine.MatchParsed(parsed)
				badge = res.Badge
				status = res.Status
				progress = res.ProgressPercent
			}

			ft.Items = append(ft.Items, FileTreeItem{
				ID:            f.ID,
				Name:          f.Name,
				CleanTitle:    parsed.DisplayTitle(),
				Size:          f.Size,
				FormattedSize: formatBytes(f.Size),
				Badge:         badge,
				Status:        status,
				Progress:      progress,
				Parsed:        parsed,
			})
		}
	} else if len(parent.UsenetFiles) > 0 {
		for _, f := range parent.UsenetFiles {
			parsed := matcher.ParseMedia(f.Name)
			var badge string
			var status matcher.WatchStatus
			var progress float64

			if matcherEngine != nil {
				res := matcherEngine.MatchParsed(parsed)
				badge = res.Badge
				status = res.Status
				progress = res.ProgressPercent
			}

			ft.Items = append(ft.Items, FileTreeItem{
				ID:            f.ID,
				Name:          f.Name,
				CleanTitle:    parsed.DisplayTitle(),
				Size:          f.Size,
				FormattedSize: formatBytes(f.Size),
				Badge:         badge,
				Status:        status,
				Progress:      progress,
				Parsed:        parsed,
			})
		}
	} else if len(parent.WebDLFiles) > 0 {
		for _, f := range parent.WebDLFiles {
			parsed := matcher.ParseMedia(f.Name)
			var badge string
			var status matcher.WatchStatus
			var progress float64

			if matcherEngine != nil {
				res := matcherEngine.MatchParsed(parsed)
				badge = res.Badge
				status = res.Status
				progress = res.ProgressPercent
			}

			ft.Items = append(ft.Items, FileTreeItem{
				ID:            f.ID,
				Name:          f.Name,
				CleanTitle:    parsed.DisplayTitle(),
				Size:          f.Size,
				FormattedSize: formatBytes(f.Size),
				Badge:         badge,
				Status:        status,
				Progress:      progress,
				Parsed:        parsed,
			})
		}
	}

	return ft
}

func (ft *FileTreeModel) MoveUp() {
	if ft.Cursor > 0 {
		ft.Cursor--
		if ft.Cursor < ft.TopIndex {
			ft.TopIndex = ft.Cursor
		}
	}
}

func (ft *FileTreeModel) MoveDown() {
	if ft.Cursor < len(ft.Items)-1 {
		ft.Cursor++
		visibleLines := ft.Height - 6
		if visibleLines > 0 && ft.Cursor >= ft.TopIndex+visibleLines {
			ft.TopIndex = ft.Cursor - visibleLines + 1
		}
	}
}

func (ft *FileTreeModel) SelectedItem() *FileTreeItem {
	if ft.Cursor >= 0 && ft.Cursor < len(ft.Items) {
		return &ft.Items[ft.Cursor]
	}
	return nil
}

func (ft *FileTreeModel) Render(theme Theme, width, height int) string {
	var sb strings.Builder

	headerTitle := "Files"
	if ft.ParentItem != nil {
		headerTitle = fmt.Sprintf("Files: %s", ft.ParentItem.CleanTitle)
	}
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorMauve).Padding(0, 1).Render(headerTitle))
	sb.WriteString("\n\n")

	if len(ft.Items) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(ColorSubtext0).Padding(2, 2).Render("No files available in this download."))
		return sb.String()
	}

	visibleLines := height - 8
	if visibleLines <= 0 {
		visibleLines = 10
	}

	endIdx := ft.TopIndex + visibleLines
	if endIdx > len(ft.Items) {
		endIdx = len(ft.Items)
	}

	for i := ft.TopIndex; i < endIdx; i++ {
		item := ft.Items[i]
		isCur := i == ft.Cursor

		cursorStr := "  "
		if isCur {
			cursorStr = "❯ "
		}

		badgeStr := "   "
		if item.Badge == "✓" {
			badgeStr = theme.BadgeWatched.Render(" ✓ ")
		} else if item.Badge == "◐" {
			badgeStr = theme.BadgeInProgress.Render(fmt.Sprintf("%2.0f%%", item.Progress))
		}

		title := item.CleanTitle
		if title == "" {
			title = item.Name
		}

		availWidth := width - 24
		if availWidth > 10 && len(title) > availWidth {
			title = title[:availWidth-3] + "..."
		}

		titleStyle := theme.ItemTitle
		if isCur {
			titleStyle = theme.ItemSelected
		}

		renderedTitle := titleStyle.Render(title)
		renderedSize := theme.ItemSize.Render(item.FormattedSize)

		line := fmt.Sprintf("%s%s %-50s  %s", cursorStr, badgeStr, renderedTitle, renderedSize)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}
