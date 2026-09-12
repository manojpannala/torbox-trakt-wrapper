package torbox_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

func TestClient_LogsRequestsWithoutLeakingTheApiKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	defer server.Close()

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := torbox.NewClient("tb-live-supersecret",
		torbox.WithBaseURL(server.URL),
		torbox.WithLogger(log),
	)

	_, err := client.GetTorrents(context.Background(), true)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "/torrents/mylist", "the request path is the point of the log")
	assert.Contains(t, out, "status=200")
	assert.NotContains(t, out, "tb-live-supersecret", "the api key must never reach the log")
}

func TestClient_DownloadLinkPathDoesNotLeakTheApiKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":"https://store.torbox.app/f.mkv?sig=x"}`))
	}))
	defer server.Close()

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client := torbox.NewClient("tb-live-supersecret",
		torbox.WithBaseURL(server.URL),
		torbox.WithLogger(log),
	)

	// RequestDownloadLink puts the api key in the query string, not a header.
	_, err := client.RequestDownloadLink(context.Background(), 1, 2, false)
	require.NoError(t, err)

	assert.NotContains(t, buf.String(), "tb-live-supersecret",
		"the api key travels in the query string on this endpoint")
	assert.Contains(t, buf.String(), "/torrents/requestdl")
}
