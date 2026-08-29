package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GenerateDeviceCode requests a new user code and verification URL from Trakt.
func (c *Client) GenerateDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("trakt client_id is required")
	}

	payload, err := json.Marshal(map[string]string{
		"client_id": c.clientID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling device code request: %w", err)
	}

	var resp DeviceCodeResponse
	if err := c.doRequest(ctx, "POST", "/oauth/device/code", bytes.NewReader(payload), &resp, false); err != nil {
		return nil, err
	}

	return &resp, nil
}

// ExchangeDeviceCode makes a single attempt to exchange the device code for OAuth tokens.
func (c *Client) ExchangeDeviceCode(ctx context.Context, deviceCode string) (*TokenResponse, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("trakt client_id is required")
	}

	payload, err := json.Marshal(map[string]string{
		"code":          deviceCode,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling device token request: %w", err)
	}

	url := fmt.Sprintf("%s/oauth/device/token", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", c.apiVersion)
	req.Header.Set("trakt-api-key", c.clientID)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var tokens TokenResponse
		if err := json.Unmarshal(respBody, &tokens); err != nil {
			return nil, fmt.Errorf("unmarshaling tokens: %w", err)
		}
		c.SetTokens(tokens)
		if c.onTokenRefreshed != nil {
			c.onTokenRefreshed(tokens)
		}
		return &tokens, nil
	case http.StatusBadRequest: // 400 Pending
		return nil, ErrDeviceCodePending
	case http.StatusNotFound: // 404 Not Found
		return nil, ErrDeviceCodeNotFound
	case http.StatusConflict: // 409 Already Used
		return nil, ErrDeviceCodeAlreadyUsed
	case http.StatusGone: // 410 Expired
		return nil, ErrDeviceCodeExpired
	case 418: // 418 Denied
		return nil, ErrDeviceCodeDenied
	case http.StatusTooManyRequests: // 429 Slow Down
		return nil, ErrDeviceCodeSlowDown
	default:
		return nil, fmt.Errorf("trakt device token error (status %d): %s", resp.StatusCode, string(respBody))
	}
}

// PollDeviceToken polls the Trakt device token endpoint until authorization is granted, expired, or cancelled.
func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string, interval int) (*TokenResponse, error) {
	if interval <= 0 {
		interval = 5
	}
	pollDuration := time.Duration(interval) * time.Second

	ticker := time.NewTicker(pollDuration)
	defer ticker.Stop()

	for {
		tokens, err := c.ExchangeDeviceCode(ctx, deviceCode)
		if err == nil {
			return tokens, nil
		}

		switch {
		case errors.Is(err, ErrDeviceCodePending):
			// keep waiting
		case errors.Is(err, ErrDeviceCodeSlowDown):
			pollDuration += 2 * time.Second
			ticker.Reset(pollDuration)
		default:
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// RefreshToken exchanges a refresh token for a new access and refresh token pair.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("trakt client_id is required")
	}

	payload, err := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"redirect_uri":  "urn:ietf:wg:oauth:2.0:oob",
		"grant_type":    "refresh_token",
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling refresh token request: %w", err)
	}

	var tokens TokenResponse
	if err := c.doRequest(ctx, "POST", "/oauth/token", bytes.NewReader(payload), &tokens, false); err != nil {
		return nil, err
	}

	c.SetTokens(tokens)
	if c.onTokenRefreshed != nil {
		c.onTokenRefreshed(tokens)
	}

	return &tokens, nil
}

func (c *Client) refreshTokenInternal(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.tokens.RefreshToken == "" {
		return errors.New("no refresh token available")
	}

	payload, err := json.Marshal(map[string]string{
		"refresh_token": c.tokens.RefreshToken,
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"redirect_uri":  "urn:ietf:wg:oauth:2.0:oob",
		"grant_type":    "refresh_token",
	})
	if err != nil {
		return fmt.Errorf("marshaling refresh token request: %w", err)
	}

	url := fmt.Sprintf("%s/oauth/token", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", c.apiVersion)
	if c.clientID != "" {
		req.Header.Set("trakt-api-key", c.clientID)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trakt refresh token failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokens TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return fmt.Errorf("decoding token response: %w", err)
	}

	c.tokens = tokens
	if c.onTokenRefreshed != nil {
		c.onTokenRefreshed(tokens)
	}

	return nil
}
