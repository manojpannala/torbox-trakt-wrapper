package trakt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestClient_HeadersAndAuth(t *testing.T) {
	var capturedAuth, capturedVersion, capturedAPIKey, capturedUA string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedVersion = r.Header.Get("trakt-api-version")
		capturedAPIKey = r.Header.Get("trakt-api-key")
		capturedUA = r.Header.Get("User-Agent")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := trakt.NewClient("test-client-id", "test-client-secret",
		trakt.WithBaseURL(server.URL),
		trakt.WithTokens(trakt.TokenResponse{
			AccessToken:  "valid-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    7776000,
			CreatedAt:    time.Now().Unix(),
		}),
		trakt.WithUserAgent("custom-trakt-agent/1.0"),
	)

	assert.Equal(t, "test-client-id", client.ClientID())
	assert.True(t, client.HasAuth())
	assert.False(t, client.IsTokenExpired())

	movies, err := client.GetWatchedMovies(context.Background())
	require.NoError(t, err)
	assert.Empty(t, movies)

	assert.Equal(t, "Bearer valid-token", capturedAuth)
	assert.Equal(t, "2", capturedVersion)
	assert.Equal(t, "test-client-id", capturedAPIKey)
	assert.Equal(t, "custom-trakt-agent/1.0", capturedUA)
}

func TestClient_ProactiveTokenRefresh(t *testing.T) {
	var refreshCalls int32
	var apiCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			atomic.AddInt32(&refreshCalls, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"access_token": "new-access-token",
				"token_type": "bearer",
				"expires_in": 7776000,
				"refresh_token": "new-refresh-token",
				"scope": "public",
				"created_at": 1724950000
			}`))
			return
		}

		if r.URL.Path == "/sync/watched/movies" {
			atomic.AddInt32(&apiCalls, 1)
			assert.Equal(t, "Bearer new-access-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	var callbackInvoked bool
	client := trakt.NewClient("cid", "csecret",
		trakt.WithBaseURL(server.URL),
		trakt.WithTokens(trakt.TokenResponse{
			AccessToken:  "expired-token",
			RefreshToken: "old-refresh-token",
			ExpiresIn:    3600,
			CreatedAt:    100, // Long expired
		}),
		trakt.WithOnTokenRefreshed(func(tokens trakt.TokenResponse) {
			callbackInvoked = true
			assert.Equal(t, "new-access-token", tokens.AccessToken)
		}),
	)

	assert.True(t, client.IsTokenExpired())

	_, err := client.GetWatchedMovies(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&refreshCalls))
	assert.Equal(t, int32(1), atomic.LoadInt32(&apiCalls))
	assert.True(t, callbackInvoked)
	assert.Equal(t, "new-access-token", client.Tokens().AccessToken)
}

func TestClient_ReactiveTokenRefreshOn401(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth/token" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"access_token": "refreshed-token",
				"token_type": "bearer",
				"expires_in": 7776000,
				"refresh_token": "refreshed-refresh-token",
				"created_at": 1724950000
			}`))
			return
		}

		if r.URL.Path == "/sync/playback" {
			count := atomic.AddInt32(&requestCount, 1)
			if count == 1 {
				// First request fails with 401
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
				return
			}
			// Second request succeeds with refreshed token
			assert.Equal(t, "Bearer refreshed-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csecret",
		trakt.WithBaseURL(server.URL),
		trakt.WithTokens(trakt.TokenResponse{
			AccessToken:  "supposedly-valid-token",
			RefreshToken: "valid-refresh-token",
			ExpiresIn:    7776000,
			CreatedAt:    time.Now().Unix(), // Looks unexpired locally
		}),
	)

	items, err := client.GetPlayback(context.Background())
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	assert.Equal(t, "refreshed-token", client.Tokens().AccessToken)
}

func TestClient_Errors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		checkErr   func(t *testing.T, err error)
	}{
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			body:       `{"message":"Not Found"}`,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, trakt.IsNotFound(err))
			},
		},
		{
			name:       "429 Rate Limit",
			statusCode: http.StatusTooManyRequests,
			body:       `{"message":"Rate Limit Exceeded"}`,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, trakt.IsRateLimited(err))
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

			client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL))
			_, err := client.GetWatchedMovies(context.Background())
			require.Error(t, err)
			tc.checkErr(t, err)
		})
	}
}

func TestClient_ErrorFormatting(t *testing.T) {
	err1 := &trakt.APIError{StatusCode: 400, Message: "Bad Request", ErrorDescription: "Field missing"}
	assert.Equal(t, "trakt api error (status 400): Bad Request - Field missing", err1.Error())

	err2 := &trakt.APIError{StatusCode: 500, Message: "Internal Server Error"}
	assert.Equal(t, "trakt api error (status 500): Internal Server Error", err2.Error())

	err3 := &trakt.APIError{StatusCode: 503}
	assert.Equal(t, "trakt api error (status 503)", err3.Error())

	assert.True(t, trakt.IsUnauthorized(trakt.ErrUnauthorized))
}
