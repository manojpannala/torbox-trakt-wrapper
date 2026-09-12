package logging_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/logging"
)

func readLog(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304
	require.NoError(t, err)
	return string(raw)
}

func TestNew_WhenNotVerboseNothingIsWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tt-wrapper.log")

	log, closeLog, err := logging.New(false, path)
	require.NoError(t, err)
	log.Debug("something happened", "detail", "value")
	require.NoError(t, closeLog())

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "a quiet run must not create a log file")
}

func TestNew_WhenVerboseDebugRecordsReachTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tt-wrapper.log")

	log, closeLog, err := logging.New(true, path)
	require.NoError(t, err)
	log.Debug("request", "method", "GET", "path", "/torrents/mylist", "status", 200)
	require.NoError(t, closeLog())

	contents := readLog(t, path)
	assert.Contains(t, contents, "/torrents/mylist")
	assert.Contains(t, contents, "status=200")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the log sits next to credentials")
}

func TestNew_SecretsAreRedacted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tt-wrapper.log")

	log, closeLog, err := logging.New(true, path)
	require.NoError(t, err)
	log.Debug("auth",
		"api_key", "tb-live-abc123",
		"access_token", "at-xyz789",
		"refresh_token", "rt-xyz789",
		"client_secret", "cs-xyz789",
		"Authorization", "Bearer tb-live-abc123",
		"path", "/oauth/token",
	)
	require.NoError(t, closeLog())

	contents := readLog(t, path)
	for _, secret := range []string{"tb-live-abc123", "at-xyz789", "rt-xyz789", "cs-xyz789"} {
		assert.NotContains(t, contents, secret, "a secret reached the log")
	}
	assert.Contains(t, contents, "/oauth/token", "non-secret context must survive")
}
