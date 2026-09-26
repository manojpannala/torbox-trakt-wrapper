package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

// TestTokenRefresh_DuringCatalogFetch_DoesNotRaceOnConfigFields builds a model
// whose Trakt token is already expired, so the catalog command's first
// request trips the automatic refresh in pkg/trakt's request path
// (trakt.Client.refreshTokenInternal, fired via WithOnTokenRefreshed). Before
// the fix, that callback wrote straight into the shared *config.Config the
// model also reads from on every render, which -race catches as soon as
// another goroutine reads a Trakt field concurrently.
func TestTokenRefresh_DuringCatalogFetch_DoesNotRaceOnConfigFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/token":
			body := fmt.Sprintf(`{"access_token":"refreshed-access","refresh_token":"refreshed-refresh","expires_in":7776000,"created_at":%d}`, time.Now().Unix())
			_, _ = w.Write([]byte(body))
		case "/sync/watched/movies", "/sync/watched/shows", "/sync/playback":
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TRAKT_BASE_URL", srv.URL)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	tomlBody := `
[torbox]
api_key = "test-api-key"

[trakt]
client_id = "test-client-id"
client_secret = "test-client-secret"
access_token = "expired-access"
refresh_token = "expired-refresh"
token_created_at = 1
token_expires_in = 10
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(tomlBody), 0600))

	cfg, err := config.LoadFromFile(cfgPath)
	require.NoError(t, err)
	require.True(t, cfg.Trakt.IsTokenExpired(), "test setup must start from an expired token")

	m := NewAppModel(context.Background(), cfg)

	done := make(chan struct{})
	go func() {
		defer close(done)
		m.fetchTraktCatalogCmd()()
	}()

racing:
	for {
		select {
		case <-done:
			break racing
		default:
			_ = m.cfg.Trakt.AccessToken
		}
	}

	reloaded, err := config.LoadFromFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "refreshed-access", reloaded.Trakt.AccessToken)
	assert.Equal(t, "refreshed-refresh", reloaded.Trakt.RefreshToken)
}
