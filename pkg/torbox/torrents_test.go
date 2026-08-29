package torbox_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
)

const sampleTorrentsJSON = `{
  "success": true,
  "detail": "Retrieved torrents successfully",
  "data": [
    {
      "id": 101,
      "hash": "abc123hash",
      "name": "Oppenheimer.2023.2160p.UHD.Remux",
      "size": 52428800000,
      "active": true,
      "download_state": "completed",
      "download_speed": 0,
      "upload_speed": 0,
      "progress": 1.0,
      "eta": 0,
      "seeds": 85,
      "peers": 12,
      "ratio": 1.25,
      "cached": true,
      "files": [
        {
          "id": 1,
          "name": "Oppenheimer.2023.2160p.mkv",
          "size": 52428800000,
          "mimetype": "video/x-matroska",
          "short_name": "Oppenheimer.mkv",
          "absolute_path": "/Oppenheimer.2023.2160p.mkv"
        }
      ]
    }
  ]
}`

const sampleSingleTorrentJSON = `{
  "success": true,
  "detail": "Retrieved torrent successfully",
  "data": {
    "id": 101,
    "hash": "abc123hash",
    "name": "Oppenheimer.2023.2160p.UHD.Remux",
    "size": 52428800000,
    "active": true,
    "download_state": "completed",
    "progress": 1.0,
    "files": []
  }
}`

func TestTorrents_GetTorrents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/torrents/mylist", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("bypass_cache"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleTorrentsJSON))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	torrents, err := client.GetTorrents(context.Background(), true)

	require.NoError(t, err)
	require.Len(t, torrents, 1)
	assert.Equal(t, 101, torrents[0].ID)
	assert.Equal(t, "Oppenheimer.2023.2160p.UHD.Remux", torrents[0].Name)
	assert.Equal(t, int64(52428800000), torrents[0].Size)
	assert.Equal(t, "completed", torrents[0].DownloadState)
	assert.Equal(t, 1.0, torrents[0].Progress)
	assert.True(t, torrents[0].Cached)
	require.Len(t, torrents[0].Files, 1)
	assert.Equal(t, "Oppenheimer.2023.2160p.mkv", torrents[0].Files[0].Name)
}

func TestTorrents_GetTorrent_Single(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/torrents/mylist", r.URL.Path)
		assert.Equal(t, "101", r.URL.Query().Get("id"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleSingleTorrentJSON))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	item, err := client.GetTorrent(context.Background(), 101, false)

	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, 101, item.ID)
	assert.Equal(t, "Oppenheimer.2023.2160p.UHD.Remux", item.Name)
}

func TestTorrents_RequestDownloadLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/torrents/requestdl", r.URL.Path)
		assert.Equal(t, "101", r.URL.Query().Get("torrent_id"))
		assert.Equal(t, "1", r.URL.Query().Get("file_id"))
		assert.Equal(t, "false", r.URL.Query().Get("zip_link"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Download link generated","data":"https://cdn.torbox.app/stream/oppenheimer.mkv?token=abc"}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	link, err := client.RequestDownloadLink(context.Background(), 101, 1, false)

	require.NoError(t, err)
	assert.Equal(t, "https://cdn.torbox.app/stream/oppenheimer.mkv?token=abc", link)
}

func TestTorrents_CreateTorrent_Magnet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/torrents/createtorrent", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), "magnet:?xt=urn:btih:test")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Torrent added","data":{"torrent_id":202,"hash":"test"}}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	resp, err := client.CreateTorrent(context.Background(), torbox.CreateTorrentRequest{
		Magnet: "magnet:?xt=urn:btih:test",
		Name:   "Test Torrent",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 202, resp.TorrentID)
	assert.Equal(t, "test", resp.Hash)
}

func TestTorrents_CreateTorrent_File(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/torrents/createtorrent", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		err := r.ParseMultipartForm(10 << 20) // #nosec G120
		require.NoError(t, err)
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		assert.Equal(t, "sample.torrent", header.Filename)
		content, _ := io.ReadAll(file)
		assert.Equal(t, "torrent-file-bytes", string(content))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Torrent uploaded","data":{"torrent_id":203,"hash":"filehash"}}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	resp, err := client.CreateTorrent(context.Background(), torbox.CreateTorrentRequest{
		TorrentFile: []byte("torrent-file-bytes"),
		FileName:    "sample.torrent",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 203, resp.TorrentID)
}

func TestTorrents_ControlTorrent(t *testing.T) {
	operations := []struct {
		call     func(c *torbox.Client) error
		expected string
	}{
		{func(c *torbox.Client) error { return c.DeleteTorrent(context.Background(), 101) }, "delete"},
		{func(c *torbox.Client) error { return c.PauseTorrent(context.Background(), 101) }, "pause"},
		{func(c *torbox.Client) error { return c.ResumeTorrent(context.Background(), 101) }, "resume"},
		{func(c *torbox.Client) error { return c.ReannounceTorrent(context.Background(), 101) }, "reannounce"},
	}

	for _, tc := range operations {
		t.Run(tc.expected, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/torrents/controltorrent", r.URL.Path)
				body, _ := io.ReadAll(r.Body)
				assert.Contains(t, string(body), `"operation":"`+tc.expected+`"`)
				assert.Contains(t, string(body), `"torrent_id":101`)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"success":true,"detail":"Operation successful"}`))
			}))
			defer server.Close()

			client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
			err := tc.call(client)
			require.NoError(t, err)
		})
	}
}
