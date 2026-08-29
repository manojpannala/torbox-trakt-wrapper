package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "torrents", cfg.TorBox.DefaultCategory)
	assert.Equal(t, 15, cfg.TorBox.CacheTTLMinutes)
	assert.Equal(t, "mpv", cfg.Player.Command)
	assert.Equal(t, 90, cfg.Player.ScrobbleThresholdPercent)
	assert.True(t, cfg.Player.EnableIPC)
	assert.Equal(t, "catppuccin-mocha", cfg.UI.Theme)
	assert.False(t, cfg.TorBox.HasAuth())
	assert.False(t, cfg.Trakt.HasAuth())
	assert.True(t, cfg.Trakt.IsTokenExpired())
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sub", "config.toml")

	cfg := DefaultConfig()
	cfg.TorBox.APIKey = "tb_live_secret12345"
	cfg.Trakt.ClientID = "trakt_client_id_abc"
	cfg.Trakt.AccessToken = "trakt_access_token_xyz"
	cfg.Trakt.TokenCreatedAt = time.Now().Unix()
	cfg.Trakt.TokenExpiresIn = 7200000

	err := cfg.SaveToFile(configPath)
	require.NoError(t, err)

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	loaded, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "tb_live_secret12345", loaded.TorBox.APIKey)
	assert.Equal(t, "trakt_client_id_abc", loaded.Trakt.ClientID)
	assert.Equal(t, "trakt_access_token_xyz", loaded.Trakt.AccessToken)
	assert.True(t, loaded.TorBox.HasAuth())
	assert.True(t, loaded.Trakt.HasAuth())
	assert.False(t, loaded.Trakt.IsTokenExpired())
}

func TestLoadNonExistentFileReturnsDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "does_not_exist.toml")

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig().TorBox.DefaultCategory, cfg.TorBox.DefaultCategory)
}

func TestEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	initial := DefaultConfig()
	initial.TorBox.APIKey = "initial_key"
	err := initial.SaveToFile(configPath)
	require.NoError(t, err)

	t.Setenv("TORBOX_API_KEY", "env_override_key")
	t.Setenv("TRAKT_ACCESS_TOKEN", "env_trakt_token")

	loaded, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "env_override_key", loaded.TorBox.APIKey)
	assert.Equal(t, "env_trakt_token", loaded.Trakt.AccessToken)
}

func TestTraktTokenExpiry(t *testing.T) {
	now := time.Now().Unix()

	t.Run("empty token is expired", func(t *testing.T) {
		trakt := TraktConfig{AccessToken: ""}
		assert.True(t, trakt.IsTokenExpired())
	})

	t.Run("valid unexpired token", func(t *testing.T) {
		trakt := TraktConfig{
			AccessToken:    "token123",
			TokenCreatedAt: now,
			TokenExpiresIn: 7200000,
		}
		assert.False(t, trakt.IsTokenExpired())
	})

	t.Run("token nearing expiry within 24h buffer", func(t *testing.T) {
		trakt := TraktConfig{
			AccessToken:    "token123",
			TokenCreatedAt: now - 3600,
			TokenExpiresIn: 3600 + 43200, // 12h remaining < 24h buffer
		}
		assert.True(t, trakt.IsTokenExpired())
	})
}

func TestMaskSecretAndStringer(t *testing.T) {
	assert.Equal(t, "<empty>", MaskSecret(""))
	assert.Equal(t, "******", MaskSecret("12345"))
	assert.Equal(t, "tb_...789", MaskSecret("tb_live_123456789"))

	cfg := DefaultConfig()
	cfg.TorBox.APIKey = "tb_live_samplekey"
	cfg.Trakt.ClientID = "trakt_id_sample"

	str := cfg.String()
	assert.NotContains(t, str, "tb_live_samplekey")
	assert.NotContains(t, str, "trakt_id_sample")
	assert.Contains(t, str, "tb_...key")
}
