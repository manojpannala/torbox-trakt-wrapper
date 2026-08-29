package torbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL   = "https://api.torbox.app/v1/api"
	DefaultTimeout   = 30 * time.Second
	DefaultUserAgent = "torbox-trakt-wrapper/0.1.0"
)

var (
	ErrUnauthorized = errors.New("torbox: unauthorized (invalid or missing API key)")
	ErrForbidden    = errors.New("torbox: forbidden (plan limit or access denied)")
	ErrNotFound     = errors.New("torbox: resource not found")
	ErrRateLimited  = errors.New("torbox: rate limit exceeded")
)

// APIError represents an error returned by the TorBox API.
type APIError struct {
	StatusCode int
	ErrorCode  string
	Detail     string
	Message    string
}

func (e *APIError) Error() string {
	if e.ErrorCode != "" && e.Detail != "" {
		return fmt.Sprintf("torbox api error (status %d): %s - %s", e.StatusCode, e.ErrorCode, e.Detail)
	}
	if e.Detail != "" {
		return fmt.Sprintf("torbox api error (status %d): %s", e.StatusCode, e.Detail)
	}
	if e.ErrorCode != "" {
		return fmt.Sprintf("torbox api error (status %d): %s", e.StatusCode, e.ErrorCode)
	}
	if e.Message != "" {
		return fmt.Sprintf("torbox api error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("torbox api error (status %d)", e.StatusCode)
}

func IsUnauthorized(err error) bool {
	if errors.Is(err, ErrUnauthorized) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.ErrorCode == "BAD_TOKEN")
}

func IsForbidden(err error) bool {
	if errors.Is(err, ErrForbidden) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.ErrorCode == "FORBIDDEN" || apiErr.ErrorCode == "PLAN_LIMIT")
}

func IsNotFound(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.ErrorCode == "NOT_FOUND")
}

func IsRateLimited(err error) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusTooManyRequests || apiErr.ErrorCode == "RATE_LIMIT")
}

// Option configures a TorBox Client.
type Option func(*Client)

// WithBaseURL overrides the default API base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

// WithRetries configures automatic retries on rate limits or server errors.
func WithRetries(maxRetries int, retryWait time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.retryWait = retryWait
	}
}

// Client is the client for interacting with the TorBox API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	userAgent  string
	maxRetries int
	retryWait  time.Duration
}

// NewClient creates a new TorBox API client.
func NewClient(apiKey string, opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	baseURL := DefaultBaseURL
	if envURL := os.Getenv("TORBOX_BASE_URL"); envURL != "" {
		baseURL = strings.TrimRight(envURL, "/")
	}

	c := &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   DefaultTimeout,
		},
		userAgent:  DefaultUserAgent,
		maxRetries: 2,
		retryWait:  500 * time.Millisecond,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// APIKey returns the configured API key.
func (c *Client) APIKey() string {
	return c.apiKey
}

// BaseURL returns the configured base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// GetUser retrieves the authenticated user's account details and plan info.
func (c *Client) GetUser(ctx context.Context, settings bool) (*User, error) {
	path := "/user/me"
	if settings {
		path += "?settings=true"
	}

	var envelope APIResponse[User]
	if err := c.doRequest(ctx, "GET", path, nil, "", &envelope); err != nil {
		return nil, err
	}

	if !envelope.Success {
		errMsg := envelope.Detail
		if envelope.Error != nil {
			errMsg = *envelope.Error + ": " + envelope.Detail
		}
		return nil, fmt.Errorf("torbox user/me error: %s", errMsg)
	}

	return &envelope.Data, nil
}


func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string, target any) error {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	var reqBodyBytes []byte
	if body != nil {
		var err error
		reqBodyBytes, err = io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("reading request body: %w", err)
		}
	}

	var lastErr error
	attempts := c.maxRetries + 1

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryWait * time.Duration(1<<(attempt-1))):
			}
		}

		var currentBody io.Reader
		if reqBodyBytes != nil {
			currentBody = bytes.NewReader(reqBodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, currentBody)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
		req.Header.Set("User-Agent", c.userAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		} else if method == http.MethodPost || method == http.MethodPut {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading response body: %w", err)
			continue
		}

		// Handle HTTP status errors
		if resp.StatusCode >= 400 {
			var apiErr APIError
			apiErr.StatusCode = resp.StatusCode

			var envelope struct {
				Success bool    `json:"success"`
				Error   *string `json:"error"`
				Detail  string  `json:"detail"`
				Message string  `json:"message"`
			}

			if err := json.Unmarshal(respBody, &envelope); err == nil {
				if envelope.Error != nil {
					apiErr.ErrorCode = *envelope.Error
				}
				apiErr.Detail = envelope.Detail
				apiErr.Message = envelope.Message
			}

			if resp.StatusCode == http.StatusTooManyRequests {
				if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
					if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
						if attempt < attempts-1 {
							select {
							case <-ctx.Done():
								return ctx.Err()
							case <-time.After(time.Duration(seconds) * time.Second):
								continue
							}
						}
					}
				}
			}

			// Retry on 5xx or 429 if retries left
			if (resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests) && attempt < attempts-1 {
				lastErr = &apiErr
				continue
			}

			return mapStatusToError(resp.StatusCode, &apiErr)
		}

		// Status is 2xx
		if target != nil {
			if err := json.Unmarshal(respBody, target); err != nil {
				return fmt.Errorf("unmarshaling response JSON: %w (body: %s)", err, string(respBody))
			}
		}

		return nil
	}

	return lastErr
}

func mapStatusToError(statusCode int, apiErr *APIError) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: %v", ErrUnauthorized, apiErr)
	case http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrForbidden, apiErr)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %v", ErrNotFound, apiErr)
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: %v", ErrRateLimited, apiErr)
	default:
		return apiErr
	}
}
