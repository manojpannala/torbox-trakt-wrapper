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

const sampleUsenetJSON = `{
  "success": true,
  "detail": "Retrieved usenet successfully",
  "data": [
    {
      "id": 301,
      "name": "Linux.ISO.Release.nzb",
      "size": 1073741824,
      "active": true,
      "download_state": "completed",
      "download_speed": 10485760,
      "progress": 1.0,
      "files": [
        {
          "id": 1,
          "name": "linux.iso",
          "size": 1073741824
        }
      ]
    }
  ]
}`

func TestUsenet_GetUsenetList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/usenet/mylist", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleUsenetJSON))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	list, err := client.GetUsenetList(context.Background(), false)

	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 301, list[0].ID)
	assert.Equal(t, "Linux.ISO.Release.nzb", list[0].Name)
}

func TestUsenet_GetUsenetItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/usenet/mylist", r.URL.Path)
		assert.Equal(t, "301", r.URL.Query().Get("id"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleUsenetJSON))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	item, err := client.GetUsenetItem(context.Background(), 301, false)

	require.NoError(t, err)
	require.NotNil(t, item)
	assert.Equal(t, 301, item.ID)
}

func TestUsenet_RequestDownloadLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/usenet/requestdl", r.URL.Path)
		assert.Equal(t, "301", r.URL.Query().Get("usenet_id"))
		assert.Equal(t, "false", r.URL.Query().Get("zip_link"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Download link generated","data":"https://cdn.torbox.app/usenet/linux.iso"}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	link, err := client.RequestUsenetDownloadLink(context.Background(), 301, 1, false)

	require.NoError(t, err)
	assert.Equal(t, "https://cdn.torbox.app/usenet/linux.iso", link)
}

func TestUsenet_CreateUsenet_Link(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/usenet/createusenetdownload", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Usenet created","data":{"usenet_id":401,"hash":"nzbhash"}}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	resp, err := client.CreateUsenet(context.Background(), torbox.CreateUsenetRequest{
		Link: "https://example.com/item.nzb",
		Name: "Test NZB",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 401, resp.UsenetID)
}

func TestUsenet_CreateUsenet_File(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/usenet/createusenetdownload", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Usenet created","data":{"usenet_id":402,"hash":"nzbhash2"}}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	resp, err := client.CreateUsenet(context.Background(), torbox.CreateUsenetRequest{
		NZBFile:  []byte("<nzb></nzb>"),
		FileName: "upload.nzb",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 402, resp.UsenetID)
}

func TestUsenet_DeleteUsenet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/usenet/controlusenetdownload", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		assert.Contains(t, string(body), `"operation":"delete"`)
		assert.Contains(t, string(body), `"usenet_id":301`)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true,"detail":"Deleted"}`))
	}))
	defer server.Close()

	client := torbox.NewClient("key", torbox.WithBaseURL(server.URL))
	err := client.DeleteUsenet(context.Background(), 301)
	require.NoError(t, err)
}
