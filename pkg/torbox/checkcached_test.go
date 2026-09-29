package torbox_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// The live shape, trimmed: only cached hashes appear under data.
const checkCachedOneHit = `{"success":true,"error":null,"detail":"Torrent cache status retrieved successfully.",
  "data":{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa":{"name":"Sample Film 2020 1080p","size":1000,"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}`

func TestCheckCached_MarksOnlyTheHashesTorBoxReturns(t *testing.T) {
	var gotPath string
	var gotBody map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write([]byte(checkCachedOneHit))
	}))
	defer server.Close()
	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))

	cached, err := client.CheckCached(context.Background(), []string{hashA, hashB})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{hashA: true, hashB: false}, cached)
	assert.Equal(t, "/torrents/checkcached?format=object&list_files=false", gotPath)
	assert.Equal(t, []string{hashA, hashB}, gotBody["hashes"])
}

func TestCheckCached_AcceptsEveryEmptyDataShape(t *testing.T) {
	for _, data := range []string{`{}`, `[]`, `null`} {
		t.Run(data, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"success":true,"error":null,"detail":"ok","data":` + data + `}`))
			}))
			defer server.Close()
			client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))

			cached, err := client.CheckCached(context.Background(), []string{hashA})

			require.NoError(t, err)
			assert.Equal(t, map[string]bool{hashA: false}, cached)
		})
	}
}

func TestCheckCached_MakesOneAttemptAndReportsRetryAfter(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"error":"RATE_LIMIT","detail":"slow down"}`))
	}))
	defer server.Close()
	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL), torbox.WithRetries(2, time.Millisecond))

	start := time.Now()
	_, err := client.CheckCached(context.Background(), []string{hashA})

	require.Error(t, err)
	assert.True(t, torbox.IsRateLimited(err))
	assert.Equal(t, 60*time.Second, torbox.RetryAfter(err))
	assert.Equal(t, int32(1), calls.Load(), "the badge checker paces retries, not the client")
	assert.Less(t, time.Since(start), 5*time.Second, "a single attempt never sleeps on Retry-After")
}

func TestCheckCached_RejectsMoreThanOneBatch(t *testing.T) {
	client := torbox.NewClient("key", torbox.WithBaseURL("http://127.0.0.1:1"))
	hashes := make([]string, torbox.MaxCheckCached+1)
	for i := range hashes {
		hashes[i] = hashA
	}

	_, err := client.CheckCached(context.Background(), hashes)

	require.Error(t, err)
}

func TestCheckCached_NoHashesMakesNoRequest(t *testing.T) {
	client := torbox.NewClient("key", torbox.WithBaseURL("http://127.0.0.1:1"))

	cached, err := client.CheckCached(context.Background(), nil)

	require.NoError(t, err)
	assert.Empty(t, cached)
}

func TestRetryAfter_ZeroForOtherErrors(t *testing.T) {
	assert.Zero(t, torbox.RetryAfter(nil))
	assert.Zero(t, torbox.RetryAfter(context.Canceled))
}
