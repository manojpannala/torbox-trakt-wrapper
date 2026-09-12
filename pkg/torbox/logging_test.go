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
