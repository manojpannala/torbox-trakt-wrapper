package cli_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/internal/cli"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

func executeCommand(args ...string) (string, error) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd := cli.GetRootCommand()
	cmd.SetArgs(args)

	err := cmd.Execute()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()

	return buf.String(), err
}

func TestCLI_Version(t *testing.T) {
	out, err := executeCommand("version")
	require.NoError(t, err)
	assert.Contains(t, out, "tt-wrapper")
}

func TestCLI_Config(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfgPath := filepath.Join(tmpDir, "config.toml")
	out, err := executeCommand("--config", cfgPath, "config", "path")
	require.NoError(t, err)
	assert.Contains(t, out, cfgPath)

	out, err = executeCommand("--config", cfgPath, "config", "init")
	require.NoError(t, err)
	assert.Contains(t, out, "Initialized default configuration")

	out, err = executeCommand("--config", cfgPath, "config")
	require.NoError(t, err)
	assert.Contains(t, out, "Config File:")
}

func TestCLI_AuthTorBox(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli-auth-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfgPath := filepath.Join(tmpDir, "config.toml")
	_ = config.DefaultConfig().SaveToFile(cfgPath)

	out, err := executeCommand("--config", cfgPath, "auth", "torbox", "test-secret-key-12345")
	require.NoError(t, err)
	assert.Contains(t, out, "TorBox API key saved successfully")

	loaded, err := config.LoadFromFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "test-secret-key-12345", loaded.TorBox.APIKey)
}

func TestCLI_ListAndAdd(t *testing.T) {
	var torrentsCalled, usenetCalled, webdlCalled, addCalled bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/torrents/mylist") {
			torrentsCalled = true
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [
					{
						"id": 101,
						"name": "Test.Movie.Alpha.2023.1080p.mkv",
						"size": 1073741824,
						"download_state": "completed",
						"progress": 1.0
					}
				]
			}`))
			return
		} else if strings.HasSuffix(r.URL.Path, "/usenet/mylist") {
			usenetCalled = true
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [
					{
						"id": 201,
						"name": "Test.Usenet.Beta.2024.1080p.mkv",
						"size": 2147483648,
						"download_state": "cached",
						"progress": 1.0
					}
				]
			}`))
			return
		} else if strings.HasSuffix(r.URL.Path, "/webdl/mylist") {
			webdlCalled = true
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": [
					{
						"id": 301,
						"name": "Test.WebDL.Gamma.2024.720p.mkv",
						"size": 734003200,
						"download_state": "completed",
						"progress": 1.0
					}
				]
			}`))
			return
		} else if strings.HasSuffix(r.URL.Path, "/torrents/createtorrent") ||
			strings.HasSuffix(r.URL.Path, "/usenet/createusenetdownload") ||
			strings.HasSuffix(r.URL.Path, "/webdl/createwebdownload") {
			addCalled = true
			_, _ = w.Write([]byte(`{
				"success": true,
				"data": {
					"torrent_id": 102,
					"usenet_id": 202,
					"webdownload_id": 302,
					"hash": "test-hash-12345"
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("TORBOX_BASE_URL", server.URL)

	tmpDir, err := os.MkdirTemp("", "cli-list-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "dummy-api-key"
	_ = cfg.SaveToFile(cfgPath)

	out, err := executeCommand("--config", cfgPath, "list", "torrents")
	require.NoError(t, err)
	assert.True(t, torrentsCalled)
	assert.Contains(t, out, "Test Movie Alpha")

	out, err = executeCommand("--config", cfgPath, "list", "torrents", "--json")
	require.NoError(t, err)
	assert.Contains(t, out, `"clean_title": "Test Movie Alpha (2023)"`)

	out, err = executeCommand("--config", cfgPath, "list", "usenet")
	require.NoError(t, err)
	assert.True(t, usenetCalled)
	assert.Contains(t, out, "Test Usenet Beta")

	out, err = executeCommand("--config", cfgPath, "list", "webdl")
	require.NoError(t, err)
	assert.True(t, webdlCalled)
	assert.Contains(t, out, "Test Gamma (2024)")

	out, err = executeCommand("--config", cfgPath, "add", "magnet:?xt=urn:btih:dummyhash")
	require.NoError(t, err)
	assert.True(t, addCalled)
	assert.Contains(t, out, "Torrent queued successfully")

	out, err = executeCommand("--config", cfgPath, "add", "https://example.com/file.nzb")
	require.NoError(t, err)
	assert.Contains(t, out, "Usenet download queued successfully")

	out, err = executeCommand("--config", cfgPath, "add", "https://example.com/direct/video.mp4")
	require.NoError(t, err)
	assert.Contains(t, out, "Web download queued successfully")
}

func TestCLI_StreamNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/torrents/mylist") {
			_, _ = w.Write([]byte(`{"success": true, "data": []}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("TORBOX_BASE_URL", server.URL)

	tmpDir, err := os.MkdirTemp("", "cli-stream-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfgPath := filepath.Join(tmpDir, "config.toml")
	cfg := config.DefaultConfig()
	cfg.TorBox.APIKey = "dummy-api-key"
	_ = cfg.SaveToFile(cfgPath)

	_, err = executeCommand("--config", cfgPath, "stream", "NonExistentMovie")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no media found matching query")
}

func TestCLI_MalformedConfigFailsWithoutOverwriting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli-badcfg-test-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	cfgPath := filepath.Join(tmpDir, "config.toml")
	broken := "[torbox]\napi_key = 'keep-me'\ncache_ttl_minutes = 15`\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(broken), 0o600))

	_, err = executeCommand("--config", cfgPath, "config")
	require.Error(t, err, "a malformed config must not be silently ignored")
	assert.Contains(t, err.Error(), cfgPath, "the error should name the offending file")

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	assert.Equal(t, broken, string(after), "the malformed config must be left untouched")
}
