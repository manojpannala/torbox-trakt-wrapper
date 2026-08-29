package tui

import (
	"fmt"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type TabType int

const (
	TabTorrents TabType = iota
	TabUsenet
	TabWebDL
)

func (t TabType) String() string {
	switch t {
	case TabTorrents:
		return "Torrents"
	case TabUsenet:
		return "Usenet"
	case TabWebDL:
		return "Web-DL"
	default:
		return "Unknown"
	}
}

type ViewType int

const (
	ViewLibrary ViewType = iota
	ViewFileTree
)

type ModalType int

const (
	ModalNone ModalType = iota
	ModalAuth
	ModalAdd
	ModalDelete
	ModalHelp
)

type LibraryItem struct {
	ID            int
	RawName       string
	CleanTitle    string
	Size          int64
	FormattedSize string
	DownloadState string
	Progress      float64
	Speed         int64
	Seeds         int
	Category      TabType
	TorrentFiles  []torbox.TorrentFile
	UsenetFiles   []torbox.UsenetFile
	WebDLFiles    []torbox.WebDLFile
	TraktBadge    string
	TraktProgress float64
	TraktSummary  string
	WatchStatus   matcher.WatchStatus
	Parsed        matcher.ParsedMedia
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

type TorrentsLoadedMsg struct {
	Torrents []torbox.Torrent
}

type UsenetLoadedMsg struct {
	Usenet []torbox.UsenetItem
}

type WebDLLoadedMsg struct {
	WebDL []torbox.WebDLItem
}

type TraktCatalogLoadedMsg struct {
	Movies   []trakt.WatchedMovie
	Shows    []trakt.WatchedShow
	Playback []trakt.PlaybackItem
}

type StreamURLResolvedMsg struct {
	URL        string
	Title      string
	Parsed     matcher.ParsedMedia
	ResumeSecs float64
}

type DeviceCodeGeneratedMsg struct {
	Code *trakt.DeviceCodeResponse
}

type TokenPollSuccessMsg struct {
	Token *trakt.TokenResponse
}

type TokenPollErrorMsg struct {
	Err error
}

type StatusMsg struct {
	Text  string
	IsErr bool
}

type RefreshDataMsg struct{}
