package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
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
}

type UIConfig struct {
	Theme              string `toml:"theme"`
	ShowUnwatchedBadge bool   `toml:"show_unwatched_badge"`
	CompactMode        bool   `toml:"compact_mode"`
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
				"--vfs-cache-max-size=5G",
				"--force-media-title=${TITLE}",
			},
			EnableIPC:                true,
			ScrobbleThresholdPercent: DefaultScrobbleThreshold,
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

func (c Config) String() string {
	return fmt.Sprintf(
		"TorBox[Category: %s, CacheTTL: %dm, Key: %s] | Trakt[Client: %s, Auth: %t, Expired: %t] | Player[%s, IPC: %t]",
		c.TorBox.DefaultCategory,
		c.TorBox.CacheTTLMinutes,
		MaskSecret(c.TorBox.APIKey),
		MaskSecret(c.Trakt.ClientID),
		c.Trakt.HasAuth(),
		c.Trakt.IsTokenExpired(),
		c.Player.Command,
		c.Player.EnableIPC,
	)
}

func Load() (*Config, error) {
	return LoadFromFile(GetConfigFile())
}

func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.path = path

	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			cfg.applyEnvOverrides()
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config TOML: %w", err)
	}

	cfg.applyEnvOverrides()
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
}

func (c *Config) Save() error {
	if c.path != "" {
		return c.SaveToFile(c.path)
	}
	return c.SaveToFile(GetConfigFile())
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

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, FilePermission); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to atomically replace config file: %w", err)
	}

	if err := os.Chmod(path, FilePermission); err != nil {
		return err
	}

	c.path = path
	return nil
}
