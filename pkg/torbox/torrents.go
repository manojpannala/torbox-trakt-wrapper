package torbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/url"
	"strconv"
)

// GetTorrents retrieves user's torrents. If id is provided, retrieves only that torrent.
func (c *Client) GetTorrents(ctx context.Context, bypassCache bool, id ...int) ([]Torrent, error) {
	q := url.Values{}
	if bypassCache {
		q.Set("bypass_cache", "true")
	}
	if len(id) > 0 && id[0] > 0 {
		q.Set("id", strconv.Itoa(id[0]))
	}

	path := "/torrents/mylist"
	if query := q.Encode(); query != "" {
		path += "?" + query
	}

	var raw json.RawMessage
	var envelope APIResponse[*json.RawMessage]
	envelope.Data = &raw

	if err := c.doRequest(ctx, "GET", path, nil, "", &envelope); err != nil {
		return nil, err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return nil, fmt.Errorf("torbox api error: %s", errMsg)
	}

	if raw == nil {
		return []Torrent{}, nil
	}

	// Attempt to unmarshal as slice first
	var list []Torrent
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}

	// Attempt to unmarshal as single item
	var single Torrent
	if err := json.Unmarshal(raw, &single); err == nil {
		return []Torrent{single}, nil
	}

	return nil, fmt.Errorf("unexpected torrents response format: %s", string(raw))
}

// GetTorrent retrieves a single torrent by ID.
func (c *Client) GetTorrent(ctx context.Context, id int, bypassCache bool) (*Torrent, error) {
	list, err := c.GetTorrents(ctx, bypassCache, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w: torrent %d not found", ErrNotFound, id)
	}
	return &list[0], nil
}

// RequestDownloadLink requests a direct CDN download or streaming URL for a specific file in a torrent.
func (c *Client) RequestDownloadLink(ctx context.Context, torrentID int, fileID int, zip bool) (string, error) {
	q := url.Values{}
	if c.apiKey != "" {
		q.Set("token", c.apiKey)
	}
	q.Set("torrent_id", strconv.Itoa(torrentID))
	if fileID > 0 {
		q.Set("file_id", strconv.Itoa(fileID))
	}
	if zip {
		q.Set("zip_link", "true")
	} else {
		q.Set("zip_link", "false")
	}
	q.Set("redirect", "false")

	path := "/torrents/requestdl?" + q.Encode()

	var envelope APIResponse[DownloadLink]
	if err := c.doRequest(ctx, "GET", path, nil, "", &envelope); err != nil {
		return "", err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return "", fmt.Errorf("torbox requestdl error: %s", errMsg)
	}

	if envelope.Data.URL == "" {
		return "", fmt.Errorf("no download url returned by torbox")
	}

	return envelope.Data.URL, nil
}

// CreateTorrent adds a new torrent by magnet URI or .torrent file upload.
func (c *Client) CreateTorrent(ctx context.Context, req CreateTorrentRequest) (*CreateTorrentResponse, error) {
	var envelope APIResponse[CreateTorrentResponse]

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if len(req.TorrentFile) > 0 {
		fileName := req.FileName
		if fileName == "" {
			fileName = "upload.torrent"
		}
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		if _, err := part.Write(req.TorrentFile); err != nil {
			return nil, fmt.Errorf("writing torrent file: %w", err)
		}
	} else if req.Magnet != "" {
		if err := writer.WriteField("magnet", req.Magnet); err != nil {
			return nil, fmt.Errorf("writing magnet field: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either Magnet or TorrentFile must be provided")
	}

	if req.Seed > 0 {
		_ = writer.WriteField("seed", strconv.Itoa(req.Seed))
	}
	if req.AllowZip {
		_ = writer.WriteField("allow_zip", "true")
	}
	if req.Name != "" {
		_ = writer.WriteField("name", req.Name)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	if err := c.doRequest(ctx, "POST", "/torrents/createtorrent", body, writer.FormDataContentType(), &envelope); err != nil {
		return nil, err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return nil, fmt.Errorf("torbox createtorrent error: %s", errMsg)
	}

	return &envelope.Data, nil
}

// ControlTorrent performs a control operation on a torrent (delete, pause, resume, reannounce).
func (c *Client) ControlTorrent(ctx context.Context, req ControlTorrentRequest) error {
	payload, err := json.Marshal(map[string]any{
		"torrent_id": req.TorrentID,
		"operation":  req.Operation,
		"all":        req.All,
	})
	if err != nil {
		return fmt.Errorf("marshaling control torrent payload: %w", err)
	}

	var envelope APIResponse[any]
	if err := c.doRequest(ctx, "POST", "/torrents/controltorrent", bytes.NewReader(payload), "application/json", &envelope); err != nil {
		return err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return fmt.Errorf("torbox controltorrent error: %s", errMsg)
	}

	return nil
}

// DeleteTorrent deletes a torrent by ID.
func (c *Client) DeleteTorrent(ctx context.Context, id int) error {
	return c.ControlTorrent(ctx, ControlTorrentRequest{
		TorrentID: id,
		Operation: "delete",
	})
}

// PauseTorrent pauses an active torrent.
func (c *Client) PauseTorrent(ctx context.Context, id int) error {
	return c.ControlTorrent(ctx, ControlTorrentRequest{
		TorrentID: id,
		Operation: "pause",
	})
}

// ResumeTorrent resumes a paused torrent.
func (c *Client) ResumeTorrent(ctx context.Context, id int) error {
	return c.ControlTorrent(ctx, ControlTorrentRequest{
		TorrentID: id,
		Operation: "resume",
	})
}

// ReannounceTorrent requests a reannounce for a torrent.
func (c *Client) ReannounceTorrent(ctx context.Context, id int) error {
	return c.ControlTorrent(ctx, ControlTorrentRequest{
		TorrentID: id,
		Operation: "reannounce",
	})
}
