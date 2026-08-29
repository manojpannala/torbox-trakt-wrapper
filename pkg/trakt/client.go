package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL            = "https://api.trakt.tv"
	DefaultAPIVersion         = "2"
	DefaultTimeout            = 30 * time.Second
	DefaultUserAgent          = "torbox-trakt-wrapper/0.1.0"
	DefaultTokenExpiryBuffer  = 86400 // 24 hours in seconds
)

var (
	ErrUnauthorized          = errors.New("trakt: unauthorized")
	ErrForbidden             = errors.New("trakt: forbidden")
	ErrNotFound              = errors.New("trakt: resource not found")
	ErrRateLimited           = errors.New("trakt: rate limit exceeded")
	ErrTokenExpired          = errors.New("trakt: token expired")
	ErrDeviceCodePending     = errors.New("trakt: authorization pending")
	ErrDeviceCodeNotFound    = errors.New("trakt: invalid device code")
	ErrDeviceCodeAlreadyUsed = errors.New("trakt: device code already used")
	ErrDeviceCodeExpired     = errors.New("trakt: device code expired")
	ErrDeviceCodeDenied      = errors.New("trakt: device authorization denied")
	ErrDeviceCodeSlowDown    = errors.New("trakt: slow down polling frequency")
)

// APIError represents an error returned by Trakt.tv API.
type APIError struct {
	StatusCode       int
	Message          string
	ErrorDescription string
}

func (e *APIError) Error() string {
	if e.ErrorDescription != "" {
		return fmt.Sprintf("trakt api error (status %d): %s - %s", e.StatusCode, e.Message, e.ErrorDescription)
	}
	if e.Message != "" {
		return fmt.Sprintf("trakt api error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("trakt api error (status %d)", e.StatusCode)
}

func IsUnauthorized(err error) bool {
	if errors.Is(err, ErrUnauthorized) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusUnauthorized
}

func IsNotFound(err error) bool {
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func IsRateLimited(err error) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests
}

// Option configures a Trakt Client.
type Option func(*Client)

// WithBaseURL overrides default base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithHTTPClient overrides default HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithTimeout sets HTTP client timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithUserAgent sets custom User-Agent header.
func WithUserAgent(userAgent string) Option {
	return func(c *Client) {
		c.userAgent = userAgent
	}
}

// WithTokens initializes the client with existing OAuth tokens.
func WithTokens(tokens TokenResponse) Option {
	return func(c *Client) {
		c.tokens = tokens
	}
}

// WithOnTokenRefreshed registers a callback invoked when tokens are refreshed.
func WithOnTokenRefreshed(fn func(tokens TokenResponse)) Option {
	return func(c *Client) {
		c.onTokenRefreshed = fn
	}
}

// Client is the client for interacting with Trakt.tv API.
type Client struct {
	clientID         string
	clientSecret     string
	baseURL          string
	apiVersion       string
	userAgent        string
	httpClient       *http.Client
	mu               sync.RWMutex
	tokens           TokenResponse
	onTokenRefreshed func(tokens TokenResponse)
}

// NewClient creates a new Trakt.tv API client.
func NewClient(clientID, clientSecret string, opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	c := &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      DefaultBaseURL,
		apiVersion:   DefaultAPIVersion,
		userAgent:    DefaultUserAgent,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   DefaultTimeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// ClientID returns the client ID.
func (c *Client) ClientID() string {
	return c.clientID
}

// BaseURL returns the base URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Tokens returns a copy of current tokens.
func (c *Client) Tokens() TokenResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tokens
}

// SetTokens updates the current tokens.
func (c *Client) SetTokens(tokens TokenResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens = tokens
}

// HasAuth returns true if an access token is present.
func (c *Client) HasAuth() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return strings.TrimSpace(c.tokens.AccessToken) != ""
}

// IsTokenExpired returns true if the token is expired or close to expiring.
func (c *Client) IsTokenExpired() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.tokens.AccessToken == "" {
		return true
	}
	if c.tokens.CreatedAt == 0 || c.tokens.ExpiresIn == 0 {
		return false
	}
	return time.Now().Unix() >= (c.tokens.CreatedAt + c.tokens.ExpiresIn - DefaultTokenExpiryBuffer)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, target any, requiresAuth bool) error {
	var reqBodyBytes []byte
	if body != nil {
		var err error
		reqBodyBytes, err = io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("reading request body: %w", err)
		}
	}

	// Proactive token refresh if auth is required and token is expired/near expiry
	if requiresAuth && c.IsTokenExpired() && c.tokens.RefreshToken != "" {
		if err := c.refreshTokenInternal(ctx); err != nil {
			// If proactive refresh fails, we will still attempt the request or return error
			_ = err
		}
	}

	executeReq := func() (int, []byte, error) {
		url := fmt.Sprintf("%s%s", c.baseURL, path)
		var currentBody io.Reader
		if reqBodyBytes != nil {
			currentBody = bytes.NewReader(reqBodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, url, currentBody)
		if err != nil {
			return 0, nil, fmt.Errorf("creating request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("trakt-api-version", c.apiVersion)
		if c.clientID != "" {
			req.Header.Set("trakt-api-key", c.clientID)
		}
		req.Header.Set("User-Agent", c.userAgent)

		if requiresAuth {
			c.mu.RLock()
			token := c.tokens.AccessToken
			c.mu.RUnlock()
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, nil, fmt.Errorf("reading response body: %w", err)
		}

		return resp.StatusCode, respBody, nil
	}

	statusCode, respBody, err := executeReq()
	if err != nil {
		return err
	}

	// Reactive token refresh on 401
	if statusCode == http.StatusUnauthorized && requiresAuth && c.tokens.RefreshToken != "" {
		if refreshErr := c.refreshTokenInternal(ctx); refreshErr == nil {
			// Retry once with new token
			statusCode, respBody, err = executeReq()
			if err != nil {
				return err
			}
		}
	}

	if statusCode >= 400 {
		var apiErr APIError
		apiErr.StatusCode = statusCode

		var errMap map[string]any
		if jsonErr := json.Unmarshal(respBody, &errMap); jsonErr == nil {
			if msg, ok := errMap["message"].(string); ok {
				apiErr.Message = msg
			}
			if desc, ok := errMap["error_description"].(string); ok {
				apiErr.ErrorDescription = desc
			} else if desc, ok := errMap["error"].(string); ok {
				apiErr.ErrorDescription = desc
			}
		} else {
			apiErr.Message = string(respBody)
		}

		switch statusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %v", ErrUnauthorized, &apiErr)
		case http.StatusForbidden:
			return fmt.Errorf("%w: %v", ErrForbidden, &apiErr)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %v", ErrNotFound, &apiErr)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%w: %v", ErrRateLimited, &apiErr)
		default:
			return &apiErr
		}
	}

	if target != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, target); err != nil {
			return fmt.Errorf("unmarshaling response JSON: %w (body: %s)", err, string(respBody))
		}
	}

	return nil
}
