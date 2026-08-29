package torbox_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/torbox"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleWebDLJSON = `{
  "success": true,
  "detail": "Retrieved web downloads successfully",
  "data": [
    {
      "id": 501,
      "name": "DirectVideo.mp4",
      "size": 2048000000,
      "active": true,
      "download_state": "completed",
      "download_speed": 5242880,
      "progress": 1.0,
      "files": [
        {
          "id": 1,
          "name": "DirectVideo.mp4",
          "size": 2048000000
        }
      ]
    }
  ]
}`

func TestWebDL_GetWebDLList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/webdl/mylist", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleWebDLJSON))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	list, err := client.GetWebDLList(context.Background(), false)

	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 501, list[0].ID)
	assert.Equal(t, "DirectVideo.mp4", list[0].Name)
}

func TestWebDL_GetWebDLItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/webdl/mylist", r.URL.Path)
		assert.Equal(t, "501", r.URL.Query().Get("id"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleWebDLJSON))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	item, err := client.GetWebDLItem(context.Background(), 501, false)

	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, 501, item.ID)
}

func TestWebDL_RequestDownloadLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/webdl/requestdl", r.URL.Path)
		assert.Equal(t, "501", r.URL.Query().Get("web_id"))
		assert.Equal(t, "false", r.URL.Query().Get("zip_link"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Download link generated","data":"https://cdn.torbox.app/webdl/video.mp4"}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	link, err := client.RequestWebDLDownloadLink(context.Background(), 501, 1, false)

	require.NoError(t, err)
	assert.Equal(t, "https://cdn.torbox.app/webdl/video.mp4", link)
}

func TestWebDL_CreateWebDL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/webdl/createwebdownload", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		decoded, _ := url.QueryUnescape(string(body))
		assert.Contains(t, decoded, "https://example.com/file.mp4")
		assert.Contains(t, decoded, "Sample Video")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"WebDL created","data":{"webdownload_id":601}}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	resp, err := client.CreateWebDL(context.Background(), torbox.CreateWebDLRequest{
		Link: "https://example.com/file.mp4",
		Name: "Sample Video",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 601, resp.WebDLID)
}

func TestWebDL_DeleteWebDL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/webdl/controlwebdownload", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"operation":"delete"`)
		assert.Contains(t, string(body), `"webdl_id":501`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Deleted"}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	err := client.DeleteWebDL(context.Background(), 501)
	require.NoError(t, err)
}
