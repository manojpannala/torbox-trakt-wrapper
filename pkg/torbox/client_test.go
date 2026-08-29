package torbox_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_OptionsAndAuth(t *testing.T) {
	var capturedAuth string
	var capturedUA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"ok","data":[]}`))
	}))
	defer server.Close()

	client := torbox.NewClient("test-key-123",
		torbox.WithBaseURL(server.URL),
		torbox.WithUserAgent("custom-agent/1.0"),
		torbox.WithTimeout(5*time.Second),
		torbox.WithRetries(1, 10*time.Millisecond),
	)

	assert.Equal(t, "test-key-123", client.APIKey())
	assert.Equal(t, server.URL, client.BaseURL())

	torrents, err := client.GetTorrents(context.Background(), false)
	require.NoError(t, err)
	assert.Empty(t, torrents)
	assert.Equal(t, "Bearer test-key-123", capturedAuth)
	assert.Equal(t, "custom-agent/1.0", capturedUA)
}

func TestClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		checkErr   func(t *testing.T, err error)
	}{
		{
			name:       "401 Unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"success":false,"error":"BAD_TOKEN","detail":"Invalid API Key"}`,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, torbox.IsUnauthorized(err))
				assert.False(t, torbox.IsForbidden(err))
			},
		},
		{
			name:       "403 Forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"success":false,"error":"PLAN_LIMIT","detail":"Plan does not allow this action"}`,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, torbox.IsForbidden(err))
				assert.False(t, torbox.IsUnauthorized(err))
			},
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			body:       `{"success":false,"error":"NOT_FOUND","detail":"Torrent not found"}`,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, torbox.IsNotFound(err))
			},
		},
		{
			name:       "429 Rate Limit",
			statusCode: http.StatusTooManyRequests,
			body:       `{"success":false,"error":"RATE_LIMIT","detail":"Rate limit exceeded"}`,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, torbox.IsRateLimited(err))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := torbox.NewClient("test-key",
				torbox.WithBaseURL(server.URL),
				torbox.WithRetries(0, 0),
			)

			_, err := client.GetTorrents(context.Background(), false)
			require.Error(t, err)
			tc.checkErr(t, err)
		})
	}
}

func TestClient_RetryOnServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"success":false,"error":"SERVICE_UNAVAILABLE"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"ok","data":[]}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key",
		torbox.WithBaseURL(server.URL),
		torbox.WithRetries(2, 5*time.Millisecond),
	)

	items, err := client.GetTorrents(context.Background(), false)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := client.GetTorrents(ctx, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}

func TestClient_GetUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/user/me", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("settings"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"success": true,
			"detail": "User found",
			"data": {
				"id": 1,
				"email": "user@example.com",
				"plan": 2,
				"total_downloaded": 1024000
			}
		}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	user, err := client.GetUser(context.Background(), true)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "user@example.com", user.Email)
	assert.Equal(t, 2, user.Plan)
}

func TestDownloadLink_Unmarshal(t *testing.T) {
	var link1 torbox.DownloadLink
	err := link1.UnmarshalJSON([]byte(`"https://example.com/stream.mkv"`))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/stream.mkv", link1.URL)

	var link2 torbox.DownloadLink
	err = link2.UnmarshalJSON([]byte(`{"url":"https://example.com/object.mkv"}`))
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/object.mkv", link2.URL)
}

func TestAPIError_Formatting(t *testing.T) {
	err1 := &torbox.APIError{StatusCode: 400, ErrorCode: "BAD_REQUEST", Detail: "Invalid field"}
	assert.Equal(t, "torbox api error (status 400): BAD_REQUEST - Invalid field", err1.Error())

	err2 := &torbox.APIError{StatusCode: 403, Detail: "Forbidden action"}
	assert.Equal(t, "torbox api error (status 403): Forbidden action", err2.Error())

	err3 := &torbox.APIError{StatusCode: 404, ErrorCode: "NOT_FOUND"}
	assert.Equal(t, "torbox api error (status 404): NOT_FOUND", err3.Error())

	err4 := &torbox.APIError{StatusCode: 500, Message: "Internal server error"}
	assert.Equal(t, "torbox api error (status 500): Internal server error", err4.Error())

	err5 := &torbox.APIError{StatusCode: 502}
	assert.Equal(t, "torbox api error (status 502)", err5.Error())

	assert.True(t, torbox.IsUnauthorized(torbox.ErrUnauthorized))
	assert.True(t, torbox.IsForbidden(torbox.ErrForbidden))
	assert.True(t, torbox.IsNotFound(torbox.ErrNotFound))
	assert.True(t, torbox.IsRateLimited(torbox.ErrRateLimited))
}

func TestClient_WithHTTPClient(t *testing.T) {
	customHC := &http.Client{Timeout: 12 * time.Second}
	client := torbox.NewClient("key", torbox.WithHTTPClient(customHC))
	assert.NotNil(t, client)
}

func TestClient_APIErrorResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":false,"detail":"Something went wrong"}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))

	_, err := client.GetTorrents(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.GetUser(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.GetUsenetList(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.GetWebDLList(context.Background(), false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.RequestDownloadLink(context.Background(), 1, 1, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.RequestUsenetDownloadLink(context.Background(), 1, 1, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.RequestWebDLDownloadLink(context.Background(), 1, 1, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.CreateTorrent(context.Background(), torbox.CreateTorrentRequest{Magnet: "magnet:?"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.CreateUsenet(context.Background(), torbox.CreateUsenetRequest{Link: "http://example.com/item.nzb"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	_, err = client.CreateWebDL(context.Background(), torbox.CreateWebDLRequest{Link: "http://example.com/file.mp4"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	err = client.DeleteTorrent(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	err = client.DeleteUsenet(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")

	err = client.DeleteWebDL(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Something went wrong")
}
