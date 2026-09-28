package tui

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func headerAfter(t *testing.T, m AppModel, msgs ...tea.Msg) string {
	t.Helper()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	for _, msg := range msgs {
		next, _ = next.(AppModel).Update(msg)
	}
	return next.(AppModel).renderHeader()
}

func TestTraktBadge(t *testing.T) {
	rejected := fmt.Errorf("%w: %v", trakt.ErrUnauthorized, &trakt.APIError{StatusCode: 401})
	offline := &url.Error{Op: "Get", URL: "https://api.trakt.tv/sync/playback", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("no route to host")}}

	t.Run("not paired", func(t *testing.T) {
		m := testModelWith(t, func(c *config.Config) { c.Trakt.AccessToken = "" })
		assert.Contains(t, headerAfter(t, m), "Trakt: [Not Paired]")
	})

	t.Run("connected", func(t *testing.T) {
		m := testModel(t)
		assert.Contains(t, headerAfter(t, m, TraktCatalogLoadedMsg{Gen: m.traktGen}), "Trakt: [Connected ✓]")
	})

	t.Run("sign-in rejected", func(t *testing.T) {
		m := testModel(t)
		h := headerAfter(t, m, TraktCatalogFailedMsg{Err: rejected, Gen: m.traktGen})
		assert.Contains(t, h, "Trakt: [Sign-in expired — press A]")
		assert.NotContains(t, h, "Connected")
	})

	t.Run("offline", func(t *testing.T) {
		m := testModel(t)
		assert.Contains(t, headerAfter(t, m, TraktCatalogFailedMsg{Err: offline, Gen: m.traktGen}), "Trakt: [Offline]")
	})

	t.Run("any other failure", func(t *testing.T) {
		m := testModel(t)
		failed := &trakt.APIError{StatusCode: 403}
		assert.Contains(t, headerAfter(t, m, TraktCatalogFailedMsg{Err: failed, Gen: m.traktGen}), "Trakt: [Unavailable]")
	})

	t.Run("a later successful fetch clears the warning", func(t *testing.T) {
		m := testModel(t)
		h := headerAfter(t, m,
			TraktCatalogFailedMsg{Err: rejected, Gen: m.traktGen},
			TraktCatalogLoadedMsg{Gen: m.traktGen},
		)
		assert.Contains(t, h, "Trakt: [Connected ✓]")
	})

	t.Run("pairing again clears the warning", func(t *testing.T) {
		m := testModel(t)
		h := headerAfter(t, m,
			TraktCatalogFailedMsg{Err: rejected, Gen: m.traktGen},
			TokenPollSuccessMsg{Token: &trakt.TokenResponse{AccessToken: "new-access", RefreshToken: "new-refresh"}},
		)
		assert.Contains(t, h, "Trakt: [Connected ✓]")
	})
}
