package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/cache"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

var (
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEscape}
	downKey  = tea.KeyPressMsg{Code: 'j', Text: "j"}
	helpKey  = tea.KeyPressMsg{Code: '?', Text: "?"}
)

func sendKey(m AppModel, k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	next, cmd := m.Update(k)
	return next.(AppModel), cmd
}

func sendMsg(m AppModel, msg tea.Msg) (AppModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(AppModel), cmd
}

func viewText(m AppModel) string {
	return strings.Join(strings.Fields(m.View().Content), " ")
}

// Alpha is cached at 40%. Beta has never been watched.
func deferModel(t *testing.T, mutate func(*config.Config)) AppModel {
	t.Helper()
	store, _ := newTestStore(t)
	cache.Write(store, cache.TorBoxTorrents, []torbox.Torrent{alpha, beta})
	cache.Write(store, cache.TraktCatalog, traktCatalog{
		Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, 40)},
	})
	m := testModelWith(t, mutate, WithCache(store))
	m, _ = sendMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func freshCatalog(pct float64) TraktCatalogLoadedMsg {
	return TraktCatalogLoadedMsg{Playback: []trakt.PlaybackItem{playbackFor("Test Feature Alpha", 2023, pct)}}
}

func TestDeferral_PromptUsesTheFreshPositionNotTheCachedOne(t *testing.T) {
	m := deferModel(t, nil)

	m, cmd := sendKey(m, enterKey)
	assert.Nil(t, cmd, "nothing may launch until Trakt answers")
	assert.Contains(t, viewText(m), "Checking your position")

	m, _ = sendMsg(m, freshCatalog(70))

	v := viewText(m)
	assert.Contains(t, v, "Resume Playback")
	assert.Contains(t, v, "about 70%")
	assert.NotContains(t, v, "40%", "the cached position was watched past on another device")
}

func TestDeferral_FailedFetchReleasesAgainstTheCache(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendKey(m, enterKey)

	m, _ = sendMsg(m, TraktCatalogFailedMsg{Err: errors.New("offline")})

	v := viewText(m)
	assert.Contains(t, v, "Resume Playback", "offline must never hang the launch")
	assert.Contains(t, v, "about 40%")
	assert.Contains(t, v, "Trakt is unreachable", "a stale cached position must say so where the user acts on it")
}

func TestDeferral_FreshCatalogDoesNotClaimTraktIsUnreachable(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendKey(m, enterKey)

	m, _ = sendMsg(m, freshCatalog(70))

	assert.NotContains(t, viewText(m), "Trakt is unreachable")
}

func TestDeferral_DoesNotHoldOnceSettled(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendMsg(m, freshCatalog(70))

	m, _ = sendKey(m, enterKey)

	v := viewText(m)
	assert.NotContains(t, v, "Checking your position")
	assert.Contains(t, v, "about 70%")
}

func TestDeferral_EscapeCancelsTheHeldLaunch(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendKey(m, enterKey)

	m, _ = sendKey(m, escKey)
	assert.NotContains(t, viewText(m), "Checking your position")

	m, cmd := sendMsg(m, freshCatalog(70))
	assert.Nil(t, cmd)
	assert.NotContains(t, viewText(m), "Resume Playback")
}

func TestDeferral_ASecondLaunchReplacesTheFirst(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendKey(m, enterKey)
	m, _ = sendKey(m, downKey)
	m, _ = sendKey(m, enterKey)

	m, cmd := sendMsg(m, freshCatalog(70))

	assert.NotContains(t, viewText(m), "Resume Playback", "Alpha's launch was replaced by Beta's")
	assert.NotNil(t, cmd, "Beta has no position, so it launches straight away")
}

func TestDeferral_NotAppliedWithoutTrakt(t *testing.T) {
	m := deferModel(t, func(c *config.Config) {
		c.Trakt.ClientID = ""
		c.Trakt.AccessToken = ""
	})

	m, _ = sendKey(m, enterKey)

	require.NotContains(t, viewText(m), "Checking your position")
	assert.Contains(t, viewText(m), "Resume Playback", "the cached position still drives the prompt")
}

func TestDeferral_NotAppliedBeforePairing(t *testing.T) {
	m := deferModel(t, func(c *config.Config) {
		c.Trakt.AccessToken = ""
	})

	m, _ = sendKey(m, enterKey)

	require.NotContains(t, viewText(m), "Checking your position")
	assert.Contains(t, viewText(m), "Resume Playback", "the cached position still drives the prompt")
}

func TestDeferral_EscapeDuringAHeldLaunchClosesTheOpenModalInstead(t *testing.T) {
	m := deferModel(t, nil)
	m, _ = sendKey(m, enterKey)

	m, _ = sendKey(m, helpKey)
	require.Equal(t, ModalHelp, m.activeModal, "the ? key must open Help for this regression to be meaningful")

	m, _ = sendKey(m, escKey)
	assert.Equal(t, ModalNone, m.activeModal, "esc should close Help")
	assert.NotNil(t, m.heldLaunch, "esc must not cancel the held launch while a modal consumes it")

	m, _ = sendMsg(m, freshCatalog(70))

	v := viewText(m)
	assert.Contains(t, v, "Resume Playback")
	assert.Contains(t, v, "about 70%")
}
