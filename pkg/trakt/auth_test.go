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

func TestAuth_GenerateDeviceCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/oauth/device/code", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"device_code": "dcode123",
			"user_code": "ABCD-1234",
			"verification_url": "https://trakt.tv/activate",
			"expires_in": 600,
			"interval": 5
		}`))
	}))
	defer server.Close()

	client := trakt.NewClient("client-id", "client-secret", trakt.WithBaseURL(server.URL))
	resp, err := client.GenerateDeviceCode(context.Background())

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "dcode123", resp.DeviceCode)
	assert.Equal(t, "ABCD-1234", resp.UserCode)
	assert.Equal(t, "https://trakt.tv/activate", resp.VerificationURL)
	assert.Equal(t, 600, resp.ExpiresIn)
	assert.Equal(t, 5, resp.Interval)
}

func TestAuth_ExchangeDeviceCode_StatusCodes(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   error
	}{
		{http.StatusBadRequest, trakt.ErrDeviceCodePending},
		{http.StatusNotFound, trakt.ErrDeviceCodeNotFound},
		{http.StatusConflict, trakt.ErrDeviceCodeAlreadyUsed},
		{http.StatusGone, trakt.ErrDeviceCodeExpired},
		{418, trakt.ErrDeviceCodeDenied},
		{http.StatusTooManyRequests, trakt.ErrDeviceCodeSlowDown},
	}

	for _, tc := range tests {
		t.Run(tc.expected.Error(), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL))
			_, err := client.ExchangeDeviceCode(context.Background(), "dcode")
			assert.ErrorIs(t, err, tc.expected)
		})
	}
}

func TestAuth_PollDeviceToken_Success(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		w.Header().Set("Content-Type", "application/json")
		if count < 2 {
			w.WriteHeader(http.StatusBadRequest) // Pending
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "polled-access-token",
			"token_type": "bearer",
			"expires_in": 7776000,
			"refresh_token": "polled-refresh-token",
			"scope": "public",
			"created_at": 1724950000
		}`))
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tokens, err := client.PollDeviceToken(ctx, "dcode", 1) // poll every 1s
	require.NoError(t, err)
	require.NotNil(t, tokens)
	assert.Equal(t, "polled-access-token", tokens.AccessToken)
	assert.Equal(t, "polled-access-token", client.Tokens().AccessToken)
}

func TestAuth_PollDeviceToken_Denied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(418) // Denied
	}))
	defer server.Close()

	client := trakt.NewClient("cid", "csec", trakt.WithBaseURL(server.URL))
	_, err := client.PollDeviceToken(context.Background(), "dcode", 1)
	assert.ErrorIs(t, err, trakt.ErrDeviceCodeDenied)
}

func TestAuth_RefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/oauth/token", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "manual-refreshed-token",
			"token_type": "bearer",
			"expires_in": 7776000,
			"refresh_token": "new-refresh-token",
			"created_at": 1724950000
		}`))
	}))
	defer server.Close()

	var callbackInvoked bool
	client := trakt.NewClient("cid", "csec",
		trakt.WithBaseURL(server.URL),
		trakt.WithOnTokenRefreshed(func(tokens trakt.TokenResponse) {
			callbackInvoked = true
		}),
	)

	tokens, err := client.RefreshToken(context.Background(), "old-refresh-token")
	require.NoError(t, err)
	assert.Equal(t, "manual-refreshed-token", tokens.AccessToken)
	assert.True(t, callbackInvoked)
}
