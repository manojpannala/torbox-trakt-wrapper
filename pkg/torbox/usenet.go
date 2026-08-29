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

// GetUsenetList retrieves user's Usenet downloads.
func (c *Client) GetUsenetList(ctx context.Context, bypassCache bool, id ...int) ([]UsenetItem, error) {
	q := url.Values{}
	if bypassCache {
		q.Set("bypass_cache", "true")
	}
	if len(id) > 0 && id[0] > 0 {
		q.Set("id", strconv.Itoa(id[0]))
	}

	path := "/usenet/mylist"
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
		return nil, fmt.Errorf("torbox usenet api error: %s", errMsg)
	}

	if raw == nil {
		return []UsenetItem{}, nil
	}

	var list []UsenetItem
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}

	var single UsenetItem
	if err := json.Unmarshal(raw, &single); err == nil {
		return []UsenetItem{single}, nil
	}

	return nil, fmt.Errorf("unexpected usenet response format: %s", string(raw))
}

// GetUsenetItem retrieves a single Usenet download by ID.
func (c *Client) GetUsenetItem(ctx context.Context, id int, bypassCache bool) (*UsenetItem, error) {
	list, err := c.GetUsenetList(ctx, bypassCache, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("%w: usenet item %d not found", ErrNotFound, id)
	}
	return &list[0], nil
}

// RequestUsenetDownloadLink requests a direct CDN download or streaming URL for a specific file in a Usenet item.
func (c *Client) RequestUsenetDownloadLink(ctx context.Context, usenetID int, fileID int, zip bool) (string, error) {
	q := url.Values{}
	if c.apiKey != "" {
		q.Set("token", c.apiKey)
	}
	q.Set("usenet_id", strconv.Itoa(usenetID))
	if fileID > 0 {
		q.Set("file_id", strconv.Itoa(fileID))
	}
	if zip {
		q.Set("zip_link", "true")
	} else {
		q.Set("zip_link", "false")
	}
	q.Set("redirect", "false")

	path := "/usenet/requestdl?" + q.Encode()

	var envelope APIResponse[DownloadLink]
	if err := c.doRequest(ctx, "GET", path, nil, "", &envelope); err != nil {
		return "", err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return "", fmt.Errorf("torbox usenet requestdl error: %s", errMsg)
	}

	if envelope.Data.URL == "" {
		return "", fmt.Errorf("no download url returned by torbox")
	}

	return envelope.Data.URL, nil
}

// CreateUsenet adds a new Usenet download via link or .nzb file upload.
func (c *Client) CreateUsenet(ctx context.Context, req CreateUsenetRequest) (*CreateUsenetResponse, error) {
	var envelope APIResponse[CreateUsenetResponse]

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if len(req.NZBFile) > 0 {
		fileName := req.FileName
		if fileName == "" {
			fileName = "upload.nzb"
		}
		part, err := writer.CreateFormFile("file", fileName)
		if err != nil {
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		if _, err := part.Write(req.NZBFile); err != nil {
			return nil, fmt.Errorf("writing nzb file: %w", err)
		}
	} else if req.Link != "" {
		if err := writer.WriteField("link", req.Link); err != nil {
			return nil, fmt.Errorf("writing link field: %w", err)
		}
	} else {
		return nil, fmt.Errorf("either Link or NZBFile must be provided")
	}

	if req.Name != "" {
		_ = writer.WriteField("name", req.Name)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	if err := c.doRequest(ctx, "POST", "/usenet/createusenetdownload", body, writer.FormDataContentType(), &envelope); err != nil {
		return nil, err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return nil, fmt.Errorf("torbox createusenet error: %s", errMsg)
	}

	return &envelope.Data, nil
}

// ControlUsenet performs a control operation on a Usenet download.
func (c *Client) ControlUsenet(ctx context.Context, req ControlUsenetRequest) error {
	payload, err := json.Marshal(map[string]any{
		"usenet_id": req.UsenetID,
		"operation": req.Operation,
		"all":       req.All,
	})
	if err != nil {
		return fmt.Errorf("marshaling control usenet payload: %w", err)
	}

	var envelope APIResponse[any]
	if err := c.doRequest(ctx, "POST", "/usenet/controlusenetdownload", bytes.NewReader(payload), "application/json", &envelope); err != nil {
		return err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return fmt.Errorf("torbox controlusenet error: %s", errMsg)
	}

	return nil
}

// DeleteUsenet deletes a Usenet item by ID.
func (c *Client) DeleteUsenet(ctx context.Context, id int) error {
	return c.ControlUsenet(ctx, ControlUsenetRequest{
		UsenetID:  id,
		Operation: "delete",
	})
}
