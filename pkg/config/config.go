package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultTorBoxCategory     = "torrents"
	DefaultCacheTTLMinutes    = 15
	DefaultPlayerCommand      = "mpv"
	DefaultScrobbleThreshold  = 90
	DefaultUITheme            = "catppuccin-mocha"
	TraktTokenExpiryBufferSec = 86400
)

type Config struct {
	TorBox TorBoxConfig `toml:"torbox"`
	Trakt  TraktConfig  `toml:"trakt"`
	Player PlayerConfig `toml:"player"`
	UI     UIConfig     `toml:"ui"`
	Search SearchConfig `toml:"search"`

	path string
}

func (c *Config) Path() string {
	return c.path
}

type TorBoxConfig struct {
	APIKey          string `toml:"api_key"`
	DefaultCategory string `toml:"default_category"`
	CacheTTLMinutes int    `toml:"cache_ttl_minutes"`
}

func (t TorBoxConfig) HasAuth() bool {
	return strings.TrimSpace(t.APIKey) != ""
}

type TraktConfig struct {
	ClientID       string `toml:"client_id"`
	ClientSecret   string `toml:"client_secret"`
	AccessToken    string `toml:"access_token"`
	RefreshToken   string `toml:"refresh_token"`
	TokenCreatedAt int64  `toml:"token_created_at"`
	TokenExpiresIn int64  `toml:"token_expires_in"`
}

func (t TraktConfig) HasAuth() bool {
	return strings.TrimSpace(t.AccessToken) != ""
}

func (t TraktConfig) IsTokenExpired() bool {
	if t.AccessToken == "" {
		return true
	}
	if t.TokenCreatedAt == 0 || t.TokenExpiresIn == 0 {
		return false
	}
	return time.Now().Unix() >= (t.TokenCreatedAt + t.TokenExpiresIn - TraktTokenExpiryBufferSec)
}

type PlayerConfig struct {
	Command                  string   `toml:"command"`
	Args                     []string `toml:"args"`
	EnableIPC                bool     `toml:"enable_ipc"`
	ScrobbleThresholdPercent int      `toml:"scrobble_threshold_percent"`
	KeepOpen                 string   `toml:"keep_open"`
	StreamProxy              bool     `toml:"stream_proxy"`
}

type UIConfig struct {
	Theme              string `toml:"theme"`
	ShowUnwatchedBadge bool   `toml:"show_unwatched_badge"`
	CompactMode        bool   `toml:"compact_mode"`
}

type SearchConfig struct {
	ProwlarrURL    string `toml:"prowlarr_url"`
	ProwlarrAPIKey string `toml:"prowlarr_api_key"`
}

// Enabled reports whether both Prowlarr settings are filled in; the URL
// itself is checked when search is used, so a bad value never blocks startup.
func (s SearchConfig) Enabled() bool {
	return strings.TrimSpace(s.ProwlarrURL) != "" && strings.TrimSpace(s.ProwlarrAPIKey) != ""
}

func DefaultConfig() *Config {
	return &Config{
		TorBox: TorBoxConfig{
			APIKey:          "",
			DefaultCategory: DefaultTorBoxCategory,
			CacheTTLMinutes: DefaultCacheTTLMinutes,
		},
		Trakt: TraktConfig{
			ClientID:       "",
			ClientSecret:   "",
			AccessToken:    "",
			RefreshToken:   "",
			TokenCreatedAt: 0,
			TokenExpiresIn: 0,
		},
		Player: PlayerConfig{
			Command: DefaultPlayerCommand,
			Args: []string{
				"--force-seekable=yes",
				"--resume-playback=no",
				"--save-position-on-quit=no",
				"--stream-lavf-o=reconnect=1,reconnect_streamed=1,reconnect_delay_max=30",
			},
			EnableIPC:                true,
			ScrobbleThresholdPercent: DefaultScrobbleThreshold,
			KeepOpen:                 "",
			StreamProxy:              true,
		},
		UI: UIConfig{
			Theme:              DefaultUITheme,
			ShowUnwatchedBadge: false,
			CompactMode:        false,
		},
	}
}

func MaskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return "<empty>"
	}
	if len(s) <= 6 {
		return "******"
	}
	return s[:3] + "..." + s[len(s)-3:]
}

// maskURL shows where a URL points, without the user info, query or
// fragment, any of which may carry a credential.
func maskURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "<empty>"
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "<invalid>"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

func (c Config) String() string {
	return fmt.Sprintf(
		"TorBox[Category: %s, CacheTTL: %dm, Key: %s] | Trakt[Client: %s, Auth: %t, Expired: %t] | Player[%s, IPC: %t] | Search[URL: %s, Key: %s]",
		c.TorBox.DefaultCategory,
		c.TorBox.CacheTTLMinutes,
		MaskSecret(c.TorBox.APIKey),
		MaskSecret(c.Trakt.ClientID),
		c.Trakt.HasAuth(),
		c.Trakt.IsTokenExpired(),
		c.Player.Command,
		c.Player.EnableIPC,
		maskURL(c.Search.ProwlarrURL),
		MaskSecret(c.Search.ProwlarrAPIKey),
	)
}

func Load() (*Config, error) {
	return LoadFromFile(GetConfigFile())
}

func LoadFromFile(path string) (*Config, error) {
	cfg, err := loadRaw(path)
	if err != nil {
		return nil, err
	}
	cfg.applyEnvOverrides()
	return cfg, nil
}

// loadRaw parses the TOML at path without env overrides, so callers that
// persist back to disk never bake a process-local env var into the file.
func loadRaw(path string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.path = path

	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config TOML at %s: %w", path, err)
	}

	return cfg, nil
}

func (c *Config) applyEnvOverrides() {
	if val := os.Getenv("TORBOX_API_KEY"); val != "" {
		c.TorBox.APIKey = val
	}
	if val := os.Getenv("TRAKT_CLIENT_ID"); val != "" {
		c.Trakt.ClientID = val
	}
	if val := os.Getenv("TRAKT_CLIENT_SECRET"); val != "" {
		c.Trakt.ClientSecret = val
	}
	if val := os.Getenv("TRAKT_ACCESS_TOKEN"); val != "" {
		c.Trakt.AccessToken = val
	}
	if val := os.Getenv("TRAKT_REFRESH_TOKEN"); val != "" {
		c.Trakt.RefreshToken = val
	}
	if val := os.Getenv("PROWLARR_API_KEY"); val != "" {
		c.Search.ProwlarrAPIKey = val
	}
}

func (c *Config) Save() error {
	if c.path != "" {
		return c.SaveToFile(c.path)
	}
	return c.SaveToFile(GetConfigFile())
}

// persist skips env overrides so an env-set secret never reaches the file.
func (c *Config) persist(apply func(*Config)) error {
	path := c.path
	if path == "" {
		path = GetConfigFile()
	}

	fresh, err := loadRaw(path)
	if err != nil {
		return err
	}

	apply(fresh)

	return fresh.SaveToFile(path)
}

func (c *Config) PersistTraktTokens(accessToken, refreshToken string, createdAt, expiresIn int64) error {
	return c.persist(func(fresh *Config) {
		fresh.Trakt.AccessToken = accessToken
		fresh.Trakt.RefreshToken = refreshToken
		fresh.Trakt.TokenCreatedAt = createdAt
		fresh.Trakt.TokenExpiresIn = expiresIn
	})
}

func (c *Config) PersistTorBoxKey(apiKey string) error {
	return c.persist(func(fresh *Config) {
		fresh.TorBox.APIKey = apiKey
	})
}

func (c *Config) SaveToFile(path string) error {
	dir := filepath.Dir(path)
	if err := EnsureSecureDir(dir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// A unique temp name per writer: a fixed one lets two processes clobber
	// each other and rename a torn file into place.
	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write temp config file: %w", err)
	}
	if err := tmp.Chmod(FilePermission); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to set temp config file permissions: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to sync temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to close temp config file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to atomically replace config file: %w", err)
	}

	c.path = path
	return nil
}
