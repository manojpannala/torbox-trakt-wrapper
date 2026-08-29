package torbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// GetWebDLList retrieves user's Web downloads.
func (c *Client) GetWebDLList(ctx context.Context, bypassCache bool, id ...int) ([]WebDLItem, error) {
	q := url.Values{}
	if bypassCache {
		q.Set("bypass_cache", "true")
	}
	if len(id) > 0 && id[0] > 0 {
		q.Set("id", strconv.Itoa(id[0]))
	}

	path := "/webdl/mylist"
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
		return nil, fmt.Errorf("torbox webdl api error: %s", errMsg)
	}

	if raw == nil {
		return []WebDLItem{}, nil
	}

	var list []WebDLItem
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}

	var single WebDLItem
	if err := json.Unmarshal(raw, &single); err == nil {
		return []WebDLItem{single}, nil
	}

	return nil, fmt.Errorf("unexpected webdl response format: %s", string(raw))
}

// GetWebDLItem retrieves a single Web download by ID.
func (c *Client) GetWebDLItem(ctx context.Context, id int, bypassCache bool) (*WebDLItem, error) {
	list, err := c.GetWebDLList(ctx, bypassCache, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w: webdl item %d not found", ErrNotFound, id)
	}
	return &list[0], nil
}

// RequestWebDLDownloadLink requests a direct CDN download or streaming URL for a specific file in a Web download.
func (c *Client) RequestWebDLDownloadLink(ctx context.Context, webdlID int, fileID int, zip bool) (string, error) {
	q := url.Values{}
	if c.apiKey != "" {
		q.Set("token", c.apiKey)
	}
	q.Set("web_id", strconv.Itoa(webdlID))
	if fileID > 0 {
		q.Set("file_id", strconv.Itoa(fileID))
	}
	if zip {
		q.Set("zip_link", "true")
	} else {
		q.Set("zip_link", "false")
	}
	q.Set("redirect", "false")

	path := "/webdl/requestdl?" + q.Encode()

	var envelope APIResponse[DownloadLink]
	if err := c.doRequest(ctx, "GET", path, nil, "", &envelope); err != nil {
		return "", err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return "", fmt.Errorf("torbox webdl requestdl error: %s", errMsg)
	}

	if envelope.Data.URL == "" {
		return "", fmt.Errorf("no download url returned by torbox")
	}

	return envelope.Data.URL, nil
}

// CreateWebDL adds a new Web download via URL.
func (c *Client) CreateWebDL(ctx context.Context, req CreateWebDLRequest) (*CreateWebDLResponse, error) {
	if req.Link == "" {
		return nil, fmt.Errorf("download link cannot be empty")
	}

	values := url.Values{}
	values.Set("link", req.Link)
	if req.Name != "" {
		values.Set("name", req.Name)
	}

	var envelope APIResponse[CreateWebDLResponse]
	if err := c.doRequest(ctx, "POST", "/webdl/createwebdownload", bytes.NewReader([]byte(values.Encode())), "application/x-www-form-urlencoded", &envelope); err != nil {
		return nil, err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return nil, fmt.Errorf("torbox createwebdownload error: %s", errMsg)
	}

	return &envelope.Data, nil
}

// ControlWebDL performs a control operation on a Web download.
func (c *Client) ControlWebDL(ctx context.Context, req ControlWebDLRequest) error {
	payload, err := json.Marshal(map[string]any{
		"webdl_id":  req.WebDLID,
		"operation": req.Operation,
		"all":       req.All,
	})
	if err != nil {
		return fmt.Errorf("marshaling control webdl payload: %w", err)
	}

	var envelope APIResponse[any]
	if err := c.doRequest(ctx, "POST", "/webdl/controlwebdownload", bytes.NewReader(payload), "application/json", &envelope); err != nil {
		return err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return fmt.Errorf("torbox controlwebdownload error: %s", errMsg)
	}

	return nil
}

// DeleteWebDL deletes a Web download item by ID.
func (c *Client) DeleteWebDL(ctx context.Context, id int) error {
	return c.ControlWebDL(ctx, ControlWebDLRequest{
		WebDLID:   id,
		Operation: "delete",
	})
}
