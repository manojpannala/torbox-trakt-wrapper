package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/trakt"
)

func TestClassifyFetchErr(t *testing.T) {
	dialErr := &url.Error{Op: "Get", URL: "https://api.torbox.app/v1/api/torrents/mylist", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}}

	for _, tc := range []struct {
		name string
		err  error
		want fetchErrKind
	}{
		{"torbox rejects the api key", fmt.Errorf("%w: %v", torbox.ErrUnauthorized, &torbox.APIError{StatusCode: 401, ErrorCode: "BAD_TOKEN"}), fetchErrAuthRejected},
		{"trakt rejects the token", fmt.Errorf("%w: %v", trakt.ErrUnauthorized, &trakt.APIError{StatusCode: 401}), fetchErrAuthRejected},
		{"the connection fails", dialErr, fetchErrOffline},
		{"the request times out", fmt.Errorf("fetching: %w", context.DeadlineExceeded), fetchErrOffline},
		{"torbox is down", &torbox.APIError{StatusCode: http.StatusBadGateway}, fetchErrOffline},
		{"trakt is down", &trakt.APIError{StatusCode: http.StatusServiceUnavailable}, fetchErrOffline},
		{"torbox refuses for another reason", fmt.Errorf("%w: %v", torbox.ErrForbidden, &torbox.APIError{StatusCode: 403, ErrorCode: "PLAN_LIMIT"}), fetchErrOther},
		{"a bad response", errors.New("unmarshaling response JSON: unexpected end"), fetchErrOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyFetchErr(tc.err))
		})
	}
}

func TestLibraryFetchFailure_RejectedKeySaysHowToFixIt(t *testing.T) {
	m := testModel(t)
	rejected := fmt.Errorf("%w: %v", torbox.ErrUnauthorized, &torbox.APIError{StatusCode: 401, ErrorCode: "BAD_TOKEN"})

	next, _ := m.Update(LibraryFetchFailedMsg{Tab: TabTorrents, Err: rejected, Gen: m.libGen[TabTorrents]})
	got := next.(AppModel)

	assert.Equal(t, torboxKeyRejectedStatus, got.statusText)
	assert.True(t, got.isStatusErr)
}

func TestLibraryFetchFailure_RejectedKeyIsNotCalledOfflineWhenCached(t *testing.T) {
	m := testModel(t)
	m.cachedAt[TabTorrents] = time.Now().Add(-5 * time.Minute)
	rejected := fmt.Errorf("%w: %v", torbox.ErrUnauthorized, &torbox.APIError{StatusCode: 401})

	next, _ := m.Update(LibraryFetchFailedMsg{Tab: TabTorrents, Err: rejected, Gen: m.libGen[TabTorrents]})
	got := next.(AppModel)

	assert.Equal(t, torboxKeyRejectedStatus, got.statusText, "the cached list stays, but the footer must say the key is the problem")
	assert.True(t, got.isStatusErr)
	assert.Zero(t, got.fetchFailed&tabBit(TabTorrents), "a rejected key is not offline")
}

func TestLibraryFetchFailure_OfflineWithoutCacheSaysOffline(t *testing.T) {
	m := testModel(t)
	offline := &url.Error{Op: "Get", URL: "https://api.torbox.app/v1/api/usenet/mylist", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("no route to host")}}

	next, _ := m.Update(LibraryFetchFailedMsg{Tab: TabUsenet, Err: offline, Gen: m.libGen[TabUsenet]})
	got := next.(AppModel)

	require.True(t, got.isStatusErr)
	assert.Equal(t, "Offline — couldn't load usenet", got.statusText)
}
