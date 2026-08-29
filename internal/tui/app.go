package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/matcher"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/player"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

type AppModel struct {
	cfg          *config.Config
	torboxClient *torbox.Client
	traktClient  *trakt.Client
	matcher      *matcher.Matcher
	player       player.Player
	theme        Theme

	width  int
	height int

	activeTab   TabType
	activeView  ViewType
	activeModal ModalType

	torrents []LibraryItem
	usenet   []LibraryItem
	webdl    []LibraryItem

	cursor   int
	topIndex int

	searchInput  textinput.Model
	searchActive bool

	spinner spinner.Model
	loading bool

	addModal  AddModal
	authModal AuthModal
	fileTree  FileTreeModel

	statusText  string
	isStatusErr bool
}

func NewAppModel(cfg *config.Config) AppModel {
	theme := DefaultTheme()

	var tbClient *torbox.Client
	if cfg.TorBox.APIKey != "" {
		tbClient = torbox.NewClient(cfg.TorBox.APIKey)
	}

	var trClient *trakt.Client
	if cfg.Trakt.ClientID != "" {
		trClient = trakt.NewClient(
			cfg.Trakt.ClientID,
			cfg.Trakt.ClientSecret,
			trakt.WithTokens(trakt.TokenResponse{
				AccessToken:  cfg.Trakt.AccessToken,
				RefreshToken: cfg.Trakt.RefreshToken,
				CreatedAt:    cfg.Trakt.TokenCreatedAt,
				ExpiresIn:    cfg.Trakt.TokenExpiresIn,
			}),
			trakt.WithOnTokenRefreshed(func(tokens trakt.TokenResponse) {
				cfg.Trakt.AccessToken = tokens.AccessToken
				cfg.Trakt.RefreshToken = tokens.RefreshToken
				cfg.Trakt.TokenCreatedAt = tokens.CreatedAt
				cfg.Trakt.TokenExpiresIn = tokens.ExpiresIn
				_ = cfg.Save()
			}),
		)
	}

	matcherEngine := matcher.NewMatcher(nil, nil, nil)
	mpvPlayer := player.NewMPVPlayer(
		player.WithExecutable(cfg.Player.Command),
		player.WithExtraArgs(cfg.Player.Args),
		player.WithIPCEnabled(cfg.Player.EnableIPC),
		player.WithScrobbler(player.NewTraktScrobbler(trClient)),
	)

	ti := textinput.New()
	ti.Placeholder = "Filter titles..."
	ti.CharLimit = 128
	ti.Width = 30

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorMauve)

	initialTab := TabTorrents
	switch cfg.TorBox.DefaultCategory {
	case "usenet":
		initialTab = TabUsenet
	case "webdl":
		initialTab = TabWebDL
	}

	return AppModel{
		cfg:          cfg,
		torboxClient: tbClient,
		traktClient:  trClient,
		matcher:      matcherEngine,
		player:       mpvPlayer,
		theme:        theme,
		activeTab:    initialTab,
		activeView:   ViewLibrary,
		activeModal:  ModalNone,
		searchInput:  ti,
		spinner:      sp,
		addModal:     NewAddModal(),
		authModal:    NewAuthModal(),
		loading:      true,
		statusText:   "Ready",
	}
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.fetchLibraryCmd(),
		m.fetchTraktCatalogCmd(),
	)
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.fileTree.Width = msg.Width
		m.fileTree.Height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.activeModal == ModalAuth {
			m.authModal.Spinner, _ = m.authModal.Spinner.Update(msg)
		}
		return m, cmd

	case StatusMsg:
		m.statusText = msg.Text
		m.isStatusErr = msg.IsErr
		m.loading = false
		return m, nil

	case TorrentsLoadedMsg:
		m.loading = false
		m.torrents = m.convertTorrents(msg.Torrents)
		m.reapplyFilter()
		return m, nil

	case UsenetLoadedMsg:
		m.loading = false
		m.usenet = m.convertUsenet(msg.Usenet)
		m.reapplyFilter()
		return m, nil

	case WebDLLoadedMsg:
		m.loading = false
		m.webdl = m.convertWebDL(msg.WebDL)
		m.reapplyFilter()
		return m, nil

	case TraktCatalogLoadedMsg:
		m.matcher.UpdateCatalog(msg.Movies, msg.Shows, msg.Playback)
		m.recalculateBadges()
		m.reapplyFilter()
		return m, nil

	case DeviceCodeGeneratedMsg:
		m.authModal.DeviceCode = msg.Code
		if msg.Code != nil {
			_ = clipboard.WriteAll(msg.Code.UserCode)
			m.authModal.Copied = true
			cmds = append(cmds, m.pollTokenCmd(msg.Code))
		}
		return m, tea.Batch(cmds...)

	case TokenPollSuccessMsg:
		m.cfg.Trakt.AccessToken = msg.Token.AccessToken
		m.cfg.Trakt.RefreshToken = msg.Token.RefreshToken
		m.cfg.Trakt.TokenCreatedAt = msg.Token.CreatedAt
		m.cfg.Trakt.TokenExpiresIn = msg.Token.ExpiresIn
		_ = m.cfg.Save()
		m.activeModal = ModalNone
		m.statusText = "Successfully paired with Trakt.tv!"
		m.isStatusErr = false
		cmds = append(cmds, m.fetchTraktCatalogCmd())
		return m, tea.Batch(cmds...)

	case TokenPollErrorMsg:
		m.authModal.StatusText = fmt.Sprintf("Pairing error: %v", msg.Err)
		return m, nil

	case StreamURLResolvedMsg:
		m.loading = false
		m.statusText = fmt.Sprintf("Launching %s in MPV...", msg.Title)
		playCmd := m.launchPlayerCmd(msg)
		return m, playCmd

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchActive {
		switch msg.String() {
		case "esc":
			m.searchActive = false
			m.searchInput.Blur()
			m.searchInput.Reset()
			m.reapplyFilter()
			return m, nil
		case "enter":
			m.searchActive = false
			m.searchInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.reapplyFilter()
			return m, cmd
		}
	}

	if m.activeModal != ModalNone {
		switch m.activeModal {
		case ModalHelp:
			if msg.String() == "esc" || msg.String() == "?" || msg.String() == "q" {
				m.activeModal = ModalNone
			}
			return m, nil

		case ModalDelete:
			switch msg.String() {
			case "y", "Y", "enter":
				m.activeModal = ModalNone
				return m, m.deleteCurrentItemCmd()
			case "n", "N", "esc", "q":
				m.activeModal = ModalNone
				return m, nil
			}
			return m, nil

		case ModalAdd:
			switch msg.String() {
			case "esc":
				m.activeModal = ModalNone
				return m, nil
			case "enter":
				val := strings.TrimSpace(m.addModal.Input.Value())
				if val != "" {
					m.activeModal = ModalNone
					return m, m.addDownloadCmd(val)
				}
				return m, nil
			default:
				var cmd tea.Cmd
				m.addModal.Input, cmd = m.addModal.Input.Update(msg)
				return m, cmd
			}

		case ModalAuth:
			if msg.String() == "esc" || msg.String() == "q" {
				m.activeModal = ModalNone
			}
			return m, nil
		}
	}

	if m.activeView == ViewFileTree {
		switch msg.String() {
		case "esc", "b", "q":
			m.activeView = ViewLibrary
			return m, nil
		case "up", "k":
			m.fileTree.MoveUp()
			return m, nil
		case "down", "j":
			m.fileTree.MoveDown()
			return m, nil
		case "enter", " ":
			selected := m.fileTree.SelectedItem()
			if selected != nil && m.fileTree.ParentItem != nil {
				return m, m.streamFileCmd(m.fileTree.ParentItem, selected.ID, selected.CleanTitle, selected.Parsed)
			}
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "?":
		m.activeModal = ModalHelp
		return m, nil

	case "tab":
		m.activeTab = (m.activeTab + 1) % 3
		m.cursor = 0
		m.topIndex = 0
		m.reapplyFilter()
		return m, nil

	case "1":
		m.activeTab = TabTorrents
		m.cursor = 0
		m.topIndex = 0
		m.reapplyFilter()
		return m, nil

	case "2":
		m.activeTab = TabUsenet
		m.cursor = 0
		m.topIndex = 0
		m.reapplyFilter()
		return m, nil

	case "3":
		m.activeTab = TabWebDL
		m.cursor = 0
		m.topIndex = 0
		m.reapplyFilter()
		return m, nil

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.topIndex {
				m.topIndex = m.cursor
			}
		}
		return m, nil

	case "down", "j":
		items := m.currentItems()
		if m.cursor < len(items)-1 {
			m.cursor++
			visibleLines := m.height - 8
			if visibleLines > 0 && m.cursor >= m.topIndex+visibleLines {
				m.topIndex = m.cursor - visibleLines + 1
			}
		}
		return m, nil

	case "g":
		m.cursor = 0
		m.topIndex = 0
		return m, nil

	case "G":
		items := m.currentItems()
		if len(items) > 0 {
			m.cursor = len(items) - 1
			visibleLines := m.height - 8
			if visibleLines > 0 && m.cursor >= visibleLines {
				m.topIndex = m.cursor - visibleLines + 1
			}
		}
		return m, nil

	case "/":
		m.searchActive = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case "r":
		m.loading = true
		m.statusText = "Refreshing library..."
		return m, tea.Batch(m.fetchLibraryCmd(), m.fetchTraktCatalogCmd())

	case "a":
		m.addModal = NewAddModal()
		clipText, _ := clipboard.ReadAll()
		if strings.HasPrefix(clipText, "magnet:?") || strings.HasPrefix(clipText, "http://") || strings.HasPrefix(clipText, "https://") {
			m.addModal.Input.SetValue(clipText)
		}
		m.activeModal = ModalAdd
		return m, textinput.Blink

	case "d", "x":
		if len(m.currentItems()) > 0 {
			m.activeModal = ModalDelete
		}
		return m, nil

	case "f", "o":
		item := m.selectedCurrentItem()
		if item != nil {
			m.fileTree = NewFileTreeModel(item, m.matcher)
			m.fileTree.Width = m.width
			m.fileTree.Height = m.height
			m.activeView = ViewFileTree
		}
		return m, nil

	case "A":
		m.authModal = NewAuthModal()
		m.activeModal = ModalAuth
		return m, m.generateDeviceCodeCmd()

	case "enter", " ":
		item := m.selectedCurrentItem()
		if item != nil {
			if len(item.TorrentFiles) > 1 || len(item.UsenetFiles) > 1 || len(item.WebDLFiles) > 1 {
				m.fileTree = NewFileTreeModel(item, m.matcher)
				m.fileTree.Width = m.width
				m.fileTree.Height = m.height
				m.activeView = ViewFileTree
				return m, nil
			}
			return m, m.streamItemCmd(item)
		}
	}

	return m, nil
}

func (m AppModel) currentItems() []LibraryItem {
	var src []LibraryItem
	switch m.activeTab {
	case TabTorrents:
		src = m.torrents
	case TabUsenet:
		src = m.usenet
	case TabWebDL:
		src = m.webdl
	}

	query := strings.ToLower(strings.TrimSpace(m.searchInput.Value()))
	if query == "" {
		return src
	}

	var filtered []LibraryItem
	for _, item := range src {
		if strings.Contains(strings.ToLower(item.CleanTitle), query) || strings.Contains(strings.ToLower(item.RawName), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (m AppModel) selectedCurrentItem() *LibraryItem {
	items := m.currentItems()
	if m.cursor >= 0 && m.cursor < len(items) {
		return &items[m.cursor]
	}
	return nil
}

func (m *AppModel) reapplyFilter() {
	items := m.currentItems()
	if m.cursor >= len(items) {
		if len(items) > 0 {
			m.cursor = len(items) - 1
		} else {
			m.cursor = 0
		}
	}
	if m.cursor < m.topIndex {
		m.topIndex = m.cursor
	}
}

func (m *AppModel) convertTorrents(items []torbox.Torrent) []LibraryItem {
	res := make([]LibraryItem, len(items))
	for i, t := range items {
		parsed := matcher.ParseMedia(t.Name)
		matchRes := m.matcher.MatchParsed(parsed)

		var badge string
		var watchStatus matcher.WatchStatus
		var progress float64
		var summary string

		if len(t.Files) > 1 {
			fileResults := m.matcher.MatchTorrentFiles(t.Files)
			folderStatus := matcher.AggregateFolderStatus(fileResults)
			badge = folderStatus.Badge
			watchStatus = folderStatus.Status
			summary = folderStatus.Summary
		} else {
			badge = matchRes.Badge
			watchStatus = matchRes.Status
			progress = matchRes.ProgressPercent
		}

		res[i] = LibraryItem{
			ID:            t.ID,
			RawName:       t.Name,
			CleanTitle:    parsed.DisplayTitle(),
			Size:          t.Size,
			FormattedSize: formatBytes(t.Size),
			DownloadState: t.DownloadState,
			Progress:      t.Progress * 100,
			Speed:         t.DownloadSpeed,
			Seeds:         t.Seeds,
			Category:      TabTorrents,
			TorrentFiles:  t.Files,
			TraktBadge:    badge,
			TraktProgress: progress,
			TraktSummary:  summary,
			WatchStatus:   watchStatus,
			Parsed:        parsed,
		}
	}
	return res
}

func (m *AppModel) convertUsenet(items []torbox.UsenetItem) []LibraryItem {
	res := make([]LibraryItem, len(items))
	for i, u := range items {
		parsed := matcher.ParseMedia(u.Name)
		matchRes := m.matcher.MatchParsed(parsed)

		res[i] = LibraryItem{
			ID:            u.ID,
			RawName:       u.Name,
			CleanTitle:    parsed.DisplayTitle(),
			Size:          u.Size,
			FormattedSize: formatBytes(u.Size),
			DownloadState: u.DownloadState,
			Progress:      u.Progress * 100,
			Speed:         u.DownloadSpeed,
			Category:      TabUsenet,
			UsenetFiles:   u.Files,
			TraktBadge:    matchRes.Badge,
			TraktProgress: matchRes.ProgressPercent,
			WatchStatus:   matchRes.Status,
			Parsed:        parsed,
		}
	}
	return res
}

func (m *AppModel) convertWebDL(items []torbox.WebDLItem) []LibraryItem {
	res := make([]LibraryItem, len(items))
	for i, w := range items {
		parsed := matcher.ParseMedia(w.Name)
		matchRes := m.matcher.MatchParsed(parsed)

		res[i] = LibraryItem{
			ID:            w.ID,
			RawName:       w.Name,
			CleanTitle:    parsed.DisplayTitle(),
			Size:          w.Size,
			FormattedSize: formatBytes(w.Size),
			DownloadState: w.DownloadState,
			Progress:      w.Progress * 100,
			Speed:         w.DownloadSpeed,
			Category:      TabWebDL,
			WebDLFiles:    w.Files,
			TraktBadge:    matchRes.Badge,
			TraktProgress: matchRes.ProgressPercent,
			WatchStatus:   matchRes.Status,
			Parsed:        parsed,
		}
	}
	return res
}

func (m *AppModel) recalculateBadges() {
	for i := range m.torrents {
		t := &m.torrents[i]
		if len(t.TorrentFiles) > 1 {
			fileResults := m.matcher.MatchTorrentFiles(t.TorrentFiles)
			folderStatus := matcher.AggregateFolderStatus(fileResults)
			t.TraktBadge = folderStatus.Badge
			t.WatchStatus = folderStatus.Status
			t.TraktSummary = folderStatus.Summary
		} else {
			res := m.matcher.MatchParsed(t.Parsed)
			t.TraktBadge = res.Badge
			t.WatchStatus = res.Status
			t.TraktProgress = res.ProgressPercent
		}
	}
}

func (m AppModel) View() string {
	if m.width == 0 {
		return "Initializing TorBox Trakt Wrapper..."
	}

	var sb strings.Builder

	header := m.renderHeader()
	sb.WriteString(header)
	sb.WriteString("\n")

	tabs := m.renderTabs()
	sb.WriteString(tabs)
	sb.WriteString("\n")

	var body string
	if m.activeView == ViewFileTree {
		body = m.fileTree.Render(m.theme, m.width, m.height)
	} else {
		body = m.renderLibraryList()
	}

	if m.activeModal != ModalNone {
		var modalView string
		switch m.activeModal {
		case ModalHelp:
			modalView = renderHelpModal(m.theme, m.width)
		case ModalDelete:
			modalView = renderDeleteModal(m.theme, m.selectedCurrentItem(), m.width)
		case ModalAdd:
			modalView = m.addModal.Render(m.theme, m.width)
		case ModalAuth:
			modalView = m.authModal.Render(m.theme, m.width)
		}
		body = lipgloss.Place(m.width, m.height-6, lipgloss.Center, lipgloss.Center, modalView)
	}

	sb.WriteString(body)

	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, sb.String(), footer)
}

func (m AppModel) renderHeader() string {
	title := m.theme.AppTitle.Render("TORBOX TRAKT WRAPPER")

	authBadge := lipgloss.NewStyle().Foreground(ColorPeach).Render("Trakt: [Not Paired]")
	if m.cfg.Trakt.HasAuth() {
		authBadge = lipgloss.NewStyle().Foreground(ColorGreen).Render("Trakt: [Connected ✓]")
	}

	spinnerView := ""
	if m.loading {
		spinnerView = m.spinner.View() + " "
	}

	headerLeft := title
	headerRight := fmt.Sprintf("%s%s", spinnerView, authBadge)

	gap := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight) - 2
	if gap < 0 {
		gap = 0
	}

	return m.theme.Header.Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Left, headerLeft, strings.Repeat(" ", gap), headerRight),
	)
}

func (m AppModel) renderTabs() string {
	var tabViews []string
	tabs := []TabType{TabTorrents, TabUsenet, TabWebDL}

	for _, t := range tabs {
		label := fmt.Sprintf("[%d] %s", t+1, t.String())
		if t == m.activeTab {
			tabViews = append(tabViews, m.theme.TabActive.Render(label))
		} else {
			tabViews = append(tabViews, m.theme.TabInactive.Render(label))
		}
	}

	tabsRow := lipgloss.JoinHorizontal(lipgloss.Left, tabViews...)
	if m.searchActive || m.searchInput.Value() != "" {
		searchBox := lipgloss.NewStyle().Padding(0, 1).Render(m.searchInput.View())
		gap := m.width - lipgloss.Width(tabsRow) - lipgloss.Width(searchBox) - 2
		if gap > 0 {
			tabsRow = lipgloss.JoinHorizontal(lipgloss.Left, tabsRow, strings.Repeat(" ", gap), searchBox)
		}
	}

	return m.theme.TabsBorder.Width(m.width).Render(tabsRow)
}

func (m AppModel) renderLibraryList() string {
	items := m.currentItems()
	if len(items) == 0 {
		emptyMsg := "No items in this category. Press 'a' to add a download or 'r' to refresh."
		if m.searchInput.Value() != "" {
			emptyMsg = "No items match your filter. Press Esc to clear filter."
		}
		return lipgloss.NewStyle().Foreground(ColorSubtext0).Padding(4, 2).Render(emptyMsg)
	}

	visibleLines := m.height - 8
	if visibleLines <= 0 {
		visibleLines = 10
	}

	endIdx := m.topIndex + visibleLines
	if endIdx > len(items) {
		endIdx = len(items)
	}

	var sb strings.Builder
	for i := m.topIndex; i < endIdx; i++ {
		item := items[i]
		isCur := i == m.cursor

		cursorStr := "  "
		if isCur {
			cursorStr = "❯ "
		}

		badgeStr := "   "
		switch item.TraktBadge {
		case "✓":
			badgeStr = m.theme.BadgeWatched.Render(" ✓ ")
		case "◐":
			if item.TraktSummary != "" {
				badgeStr = m.theme.BadgeInProgress.Render(item.TraktSummary)
			} else {
				badgeStr = m.theme.BadgeInProgress.Render(fmt.Sprintf("%2.0f%%", item.TraktProgress))
			}
		}

		title := item.CleanTitle
		if title == "" {
			title = item.RawName
		}

		availWidth := m.width - 36
		if availWidth > 10 && len(title) > availWidth {
			title = title[:availWidth-3] + "..."
		}

		titleStyle := m.theme.ItemTitle
		if isCur {
			titleStyle = m.theme.ItemSelected
		}

		renderedTitle := titleStyle.Render(title)
		renderedSize := m.theme.ItemSize.Render(fmt.Sprintf("%9s", item.FormattedSize))

		statusStr := item.DownloadState
		if statusStr == "downloading" {
			statusStr = fmt.Sprintf("%.0f%%", item.Progress)
		}
		renderedStatus := m.theme.ItemStatusOk.Render(fmt.Sprintf("%-10s", statusStr))

		line := fmt.Sprintf("%s%s %-50s  %s  %s", cursorStr, badgeStr, renderedTitle, renderedSize, renderedStatus)
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m AppModel) renderFooter() string {
	shortcuts := "[Tab] Switch  [Enter] Stream  [f] Files  [/] Filter  [a] Add  [d] Delete  [A] Trakt  [?] Help  [q] Quit"

	status := m.statusText
	if m.isStatusErr {
		status = m.theme.StatusError.Render("✖ " + status)
	}

	gap := m.width - lipgloss.Width(status) - lipgloss.Width(shortcuts) - 4
	if gap < 0 {
		gap = 0
	}

	return m.theme.StatusBar.Width(m.width).Render(
		lipgloss.JoinHorizontal(lipgloss.Left, status, strings.Repeat(" ", gap), shortcuts),
	)
}

func (m AppModel) fetchLibraryCmd() tea.Cmd {
	return func() tea.Msg {
		if m.torboxClient == nil {
			return StatusMsg{Text: "TorBox API key not configured", IsErr: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		switch m.activeTab {
		case TabTorrents:
			torrents, err := m.torboxClient.GetTorrents(ctx, true)
			if err != nil {
				return StatusMsg{Text: fmt.Sprintf("Failed to load torrents: %v", err), IsErr: true}
			}
			return TorrentsLoadedMsg{Torrents: torrents}
		case TabUsenet:
			usenet, err := m.torboxClient.GetUsenetList(ctx, true)
			if err != nil {
				return StatusMsg{Text: fmt.Sprintf("Failed to load usenet: %v", err), IsErr: true}
			}
			return UsenetLoadedMsg{Usenet: usenet}
		case TabWebDL:
			webdl, err := m.torboxClient.GetWebDLList(ctx, true)
			if err != nil {
				return StatusMsg{Text: fmt.Sprintf("Failed to load webdl: %v", err), IsErr: true}
			}
			return WebDLLoadedMsg{WebDL: webdl}
		}
		return nil
	}
}

func (m AppModel) fetchTraktCatalogCmd() tea.Cmd {
	return func() tea.Msg {
		if m.traktClient == nil || !m.cfg.Trakt.HasAuth() {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		movies, _ := m.traktClient.GetWatchedMovies(ctx)
		shows, _ := m.traktClient.GetWatchedShows(ctx)
		playback, _ := m.traktClient.GetPlayback(ctx)

		return TraktCatalogLoadedMsg{
			Movies:   movies,
			Shows:    shows,
			Playback: playback,
		}
	}
}

func (m AppModel) streamItemCmd(item *LibraryItem) tea.Cmd {
	return func() tea.Msg {
		if m.torboxClient == nil {
			return StatusMsg{Text: "TorBox API key not configured", IsErr: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var fileID int
		if len(item.TorrentFiles) > 0 {
			fileID = item.TorrentFiles[0].ID
		}

		var link string
		var err error

		switch item.Category {
		case TabTorrents:
			link, err = m.torboxClient.RequestDownloadLink(ctx, item.ID, fileID, false)
		case TabUsenet:
			link, err = m.torboxClient.RequestUsenetDownloadLink(ctx, item.ID, fileID, false)
		case TabWebDL:
			link, err = m.torboxClient.RequestWebDLDownloadLink(ctx, item.ID, fileID, false)
		}

		if err != nil {
			return StatusMsg{Text: fmt.Sprintf("Failed to resolve stream link: %v", err), IsErr: true}
		}

		return StreamURLResolvedMsg{
			URL:        link,
			Title:      item.CleanTitle,
			Parsed:     item.Parsed,
			ResumeSecs: item.TraktProgress,
		}
	}
}

func (m AppModel) streamFileCmd(parent *LibraryItem, fileID int, title string, parsed matcher.ParsedMedia) tea.Cmd {
	return func() tea.Msg {
		if m.torboxClient == nil {
			return StatusMsg{Text: "TorBox API key not configured", IsErr: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var link string
		var err error

		switch parent.Category {
		case TabTorrents:
			link, err = m.torboxClient.RequestDownloadLink(ctx, parent.ID, fileID, false)
		case TabUsenet:
			link, err = m.torboxClient.RequestUsenetDownloadLink(ctx, parent.ID, fileID, false)
		case TabWebDL:
			link, err = m.torboxClient.RequestWebDLDownloadLink(ctx, parent.ID, fileID, false)
		}

		if err != nil {
			return StatusMsg{Text: fmt.Sprintf("Failed to resolve stream link: %v", err), IsErr: true}
		}

		return StreamURLResolvedMsg{
			URL:    link,
			Title:  title,
			Parsed: parsed,
		}
	}
}

const outputTailLimit = 8 << 10

type outputTail struct {
	mu  sync.Mutex
	buf []byte
}

func (w *outputTail) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > outputTailLimit {
		w.buf = w.buf[len(w.buf)-outputTailLimit:]
	}
	return len(p), nil
}

func (w *outputTail) errorLine() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	var fallback string
	for _, line := range strings.FieldsFunc(string(w.buf), func(r rune) bool {
		return r == '\n' || r == '\r'
	}) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Exiting...") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "error") {
			return truncateRunes(line, 100)
		}
		fallback = line
	}
	return truncateRunes(fallback, 100)
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "\u2026"
}

type playerExec struct {
	player player.Player
	media  player.MediaStream
	tail   *outputTail
}

func (e *playerExec) SetStdin(r io.Reader) {
	e.media.Stdin = r
}

func (e *playerExec) SetStdout(w io.Writer) {
	e.media.Stdout = io.MultiWriter(w, e.tail)
}

func (e *playerExec) SetStderr(w io.Writer) {
	e.media.Stderr = io.MultiWriter(w, e.tail)
}

func (e *playerExec) Run() error {
	session, err := e.player.Play(context.Background(), e.media)
	if err != nil {
		return err
	}
	return session.Wait()
}

func (m AppModel) launchPlayerCmd(msg StreamURLResolvedMsg) tea.Cmd {
	exe := m.cfg.Player.Command
	if exe == "" {
		exe = "mpv"
	}

	if m.player == nil {
		return func() tea.Msg {
			return StatusMsg{Text: "No player configured", IsErr: true}
		}
	}

	tail := &outputTail{}
	e := &playerExec{
		player: m.player,
		tail:   tail,
		media: player.MediaStream{
			URL:        msg.URL,
			Title:      msg.Title,
			Parsed:     msg.Parsed,
			ResumeSecs: msg.ResumeSecs,
		},
	}

	return tea.Exec(e, func(err error) tea.Msg {
		if err != nil {
			if detail := tail.errorLine(); detail != "" {
				return StatusMsg{Text: fmt.Sprintf("%s failed: %s", exe, detail), IsErr: true}
			}
			return StatusMsg{Text: fmt.Sprintf("%s playback ended with error: %v", exe, err), IsErr: true}
		}
		return StatusMsg{Text: "Playback finished", IsErr: false}
	})
}

func (m AppModel) deleteCurrentItemCmd() tea.Cmd {
	item := m.selectedCurrentItem()
	if item == nil {
		return nil
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		switch item.Category {
		case TabTorrents:
			err = m.torboxClient.DeleteTorrent(ctx, item.ID)
		case TabUsenet:
			err = m.torboxClient.DeleteUsenet(ctx, item.ID)
		case TabWebDL:
			err = m.torboxClient.DeleteWebDL(ctx, item.ID)
		}

		if err != nil {
			return StatusMsg{Text: fmt.Sprintf("Failed to delete item: %v", err), IsErr: true}
		}
		return StatusMsg{Text: "Item deleted successfully", IsErr: false}
	}
}

func (m AppModel) addDownloadCmd(link string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var err error
		switch m.activeTab {
		case TabTorrents:
			_, err = m.torboxClient.CreateTorrent(ctx, torbox.CreateTorrentRequest{Magnet: link})
		case TabUsenet:
			_, err = m.torboxClient.CreateUsenet(ctx, torbox.CreateUsenetRequest{Link: link})
		case TabWebDL:
			_, err = m.torboxClient.CreateWebDL(ctx, torbox.CreateWebDLRequest{Link: link})
		}

		if err != nil {
			return StatusMsg{Text: fmt.Sprintf("Failed to add download: %v", err), IsErr: true}
		}
		return StatusMsg{Text: "Download added successfully", IsErr: false}
	}
}

func (m AppModel) generateDeviceCodeCmd() tea.Cmd {
	return func() tea.Msg {
		if m.traktClient == nil {
			return StatusMsg{Text: "Trakt client not initialized (missing client_id)", IsErr: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := m.traktClient.GenerateDeviceCode(ctx)
		if err != nil {
			return StatusMsg{Text: fmt.Sprintf("Failed to generate Trakt device code: %v", err), IsErr: true}
		}
		return DeviceCodeGeneratedMsg{Code: resp}
	}
}

func (m AppModel) pollTokenCmd(code *trakt.DeviceCodeResponse) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(code.ExpiresIn)*time.Second)
		defer cancel()

		token, err := m.traktClient.PollDeviceToken(ctx, code.DeviceCode, code.Interval)
		if err != nil {
			return TokenPollErrorMsg{Err: err}
		}
		return TokenPollSuccessMsg{Token: token}
	}
}
