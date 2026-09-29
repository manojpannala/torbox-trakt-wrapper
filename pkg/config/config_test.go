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

func TestConfig_SaveWritesBackToLoadedPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg"))

	cfgPath := filepath.Join(dir, "custom.toml")
	c := DefaultConfig()
	c.Trakt.ClientID = "real-client-id"
	require.NoError(t, c.SaveToFile(cfgPath))

	loaded, err := LoadFromFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, cfgPath, loaded.Path())

	loaded.Trakt.AccessToken = "token-from-auth"
	require.NoError(t, loaded.Save())

	reloaded, err := LoadFromFile(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "token-from-auth", reloaded.Trakt.AccessToken)

	_, err = os.Stat(GetConfigFile())
	assert.True(t, os.IsNotExist(err), "Save() must not touch the default config location")
}

func TestConfig_SaveFallsBackToDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	c := DefaultConfig()
	c.Trakt.ClientID = "from-defaults"
	require.NoError(t, c.Save())

	loaded, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "from-defaults", loaded.Trakt.ClientID)
}

func TestDefaultConfig_PlayerArgsHaveNoUnexpandedPlaceholders(t *testing.T) {
	for _, arg := range DefaultConfig().Player.Args {
		assert.NotContains(t, arg, "${", "nothing expands placeholders in player args: %s", arg)
	}
}

func TestPersistTraktTokens_KeepsConcurrentDiskEditsToOtherKeys(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	initial := DefaultConfig()
	initial.TorBox.APIKey = "original-key"
	require.NoError(t, initial.SaveToFile(configPath))

	a, err := LoadFromFile(configPath)
	require.NoError(t, err)

	rewritten := DefaultConfig()
	rewritten.TorBox.APIKey = "rewritten-by-someone-else"
	require.NoError(t, rewritten.SaveToFile(configPath))

	require.NoError(t, a.PersistTraktTokens("new-access", "new-refresh", 1000, 7200))

	onDisk, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "rewritten-by-someone-else", onDisk.TorBox.APIKey)
	assert.Equal(t, "new-access", onDisk.Trakt.AccessToken)
	assert.Equal(t, "new-refresh", onDisk.Trakt.RefreshToken)
	assert.EqualValues(t, 1000, onDisk.Trakt.TokenCreatedAt)
	assert.EqualValues(t, 7200, onDisk.Trakt.TokenExpiresIn)
}

func TestPersistTraktTokens_DoesNotWriteEnvOverridesToDisk(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	initial := DefaultConfig()
	initial.TorBox.APIKey = "file-key"
	require.NoError(t, initial.SaveToFile(configPath))

	t.Setenv("TORBOX_API_KEY", "env-key-must-not-leak-to-disk")

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "env-key-must-not-leak-to-disk", cfg.TorBox.APIKey, "env override applies in memory")

	require.NoError(t, cfg.PersistTraktTokens("new-access", "new-refresh", 1000, 7200))

	// loadRaw skips env overrides, so this reflects exactly what's on disk.
	onDisk, err := loadRaw(configPath)
	require.NoError(t, err)
	assert.Equal(t, "file-key", onDisk.TorBox.APIKey)

	raw, err := os.ReadFile(configPath) // #nosec G304
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "env-key-must-not-leak-to-disk")
}

func TestPersistTraktTokens_NeverMutatesTheReceiver(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.TorBox.APIKey = "receiver-key"
	require.NoError(t, cfg.SaveToFile(configPath))

	before := *cfg
	require.NoError(t, cfg.PersistTraktTokens("new-access", "new-refresh", 1000, 7200))

	assert.Equal(t, before.Trakt, cfg.Trakt, "PersistTraktTokens must not mutate the receiver's fields")
	assert.Equal(t, before.TorBox, cfg.TorBox)
}

func TestPersistTorBoxKey_KeepsConcurrentDiskEditsToOtherKeys(t *testing.T) {
	for _, name := range []string{"TORBOX_API_KEY", "TRAKT_CLIENT_ID", "TRAKT_CLIENT_SECRET", "TRAKT_ACCESS_TOKEN", "TRAKT_REFRESH_TOKEN"} {
		t.Setenv(name, "")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	initial := DefaultConfig()
	initial.Trakt.AccessToken = "original-token"
	require.NoError(t, initial.SaveToFile(configPath))

	a, err := LoadFromFile(configPath)
	require.NoError(t, err)

	rewritten := DefaultConfig()
	rewritten.Trakt.AccessToken = "rewritten-by-someone-else"
	require.NoError(t, rewritten.SaveToFile(configPath))

	require.NoError(t, a.PersistTorBoxKey("new-api-key"))

	onDisk, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "rewritten-by-someone-else", onDisk.Trakt.AccessToken)
	assert.Equal(t, "new-api-key", onDisk.TorBox.APIKey)
}

func TestPersistTorBoxKey_DoesNotWriteEnvOverridesToDisk(t *testing.T) {
	for _, name := range []string{"TORBOX_API_KEY", "TRAKT_CLIENT_ID", "TRAKT_ACCESS_TOKEN", "TRAKT_REFRESH_TOKEN"} {
		t.Setenv(name, "")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	initial := DefaultConfig()
	initial.Trakt.ClientSecret = "file-secret"
	require.NoError(t, initial.SaveToFile(configPath))

	t.Setenv("TRAKT_CLIENT_SECRET", "env-secret-must-not-leak-to-disk")

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "env-secret-must-not-leak-to-disk", cfg.Trakt.ClientSecret, "env override applies in memory")

	require.NoError(t, cfg.PersistTorBoxKey("new-api-key"))

	onDisk, err := loadRaw(configPath)
	require.NoError(t, err)
	assert.Equal(t, "file-secret", onDisk.Trakt.ClientSecret)

	raw, err := os.ReadFile(configPath) // #nosec G304
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "env-secret-must-not-leak-to-disk")
}

func TestPersistTorBoxKey_NeverMutatesTheReceiver(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	cfg.TorBox.APIKey = "receiver-key"
	require.NoError(t, cfg.SaveToFile(configPath))

	before := *cfg
	require.NoError(t, cfg.PersistTorBoxKey("new-api-key"))

	assert.Equal(t, before.TorBox, cfg.TorBox, "PersistTorBoxKey must not mutate the receiver's fields")
	assert.Equal(t, before.Trakt, cfg.Trakt)
}

func TestPersistTraktTokens_FileModeAndNoLeftoverTempFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	require.NoError(t, cfg.SaveToFile(configPath))

	require.NoError(t, cfg.PersistTraktTokens("new-access", "new-refresh", 1000, 7200))

	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp", "no leftover temp file after PersistTraktTokens")
	}
}

func TestStreamProxy_OnByDefault(t *testing.T) {
	assert.True(t, DefaultConfig().Player.StreamProxy)
}

func TestStreamProxy_ConfigWrittenBeforeTheSettingExistedKeepsItOn(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[player]\ncommand = \"mpv\"\n"), 0o600))

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.True(t, cfg.Player.StreamProxy)
}

func TestStreamProxy_CanBeTurnedOff(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[player]\nstream_proxy = false\n"), 0o600))

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.False(t, cfg.Player.StreamProxy)
}

func TestSearch_OffByDefault(t *testing.T) {
	assert.False(t, DefaultConfig().Search.Enabled())
}

func TestSearch_NeedsBothTheURLAndTheKey(t *testing.T) {
	assert.False(t, SearchConfig{ProwlarrURL: "http://127.0.0.1:9696"}.Enabled())
	assert.False(t, SearchConfig{ProwlarrAPIKey: "k"}.Enabled())
	assert.True(t, SearchConfig{ProwlarrURL: "http://127.0.0.1:9696", ProwlarrAPIKey: "k"}.Enabled())
}

func TestSearch_LoadsFromTheSearchTable(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	body := "[search]\nprowlarr_url = \"http://127.0.0.1:9696\"\nprowlarr_api_key = \"file_key\"\n"
	require.NoError(t, os.WriteFile(configPath, []byte(body), 0o600))

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:9696", cfg.Search.ProwlarrURL)
	assert.Equal(t, "file_key", cfg.Search.ProwlarrAPIKey)
}

func TestSearch_EnvKeyOverridesTheFileButIsNeverPersisted(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[search]\nprowlarr_api_key = \"file_key\"\n"), 0o600))
	t.Setenv("PROWLARR_API_KEY", "env_prowlarr_key")

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, "env_prowlarr_key", cfg.Search.ProwlarrAPIKey)

	require.NoError(t, cfg.PersistTorBoxKey("tb_new"))
	data, err := os.ReadFile(configPath) // #nosec G304
	require.NoError(t, err)
	assert.NotContains(t, string(data), "env_prowlarr_key")
	assert.Contains(t, string(data), "file_key")
}

func TestSearch_StringerMasksTheProwlarrKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Search.ProwlarrAPIKey = "prowlarr_secret_value"

	str := cfg.String()
	assert.NotContains(t, str, "prowlarr_secret_value")
	assert.Contains(t, str, "pro...lue")
}
