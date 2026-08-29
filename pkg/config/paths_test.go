package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathResolvers(t *testing.T) {
	t.Run("default paths without XDG", func(t *testing.T) {
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")

		cfgDir := GetConfigDir()
		cfgFile := GetConfigFile()
		cacheDir := GetCacheDir()

		assert.True(t, strings.HasSuffix(cfgDir, filepath.Join(".config", AppName)))
		assert.Equal(t, filepath.Join(cfgDir, "config.toml"), cfgFile)
		assert.True(t, strings.HasSuffix(cacheDir, filepath.Join(".cache", AppName)))
	})

	t.Run("paths with XDG environment variables", func(t *testing.T) {
		tmpDir := t.TempDir()
		xdgConfig := filepath.Join(tmpDir, "custom_config")
		xdgCache := filepath.Join(tmpDir, "custom_cache")

		t.Setenv("XDG_CONFIG_HOME", xdgConfig)
		t.Setenv("XDG_CACHE_HOME", xdgCache)

		assert.Equal(t, filepath.Join(xdgConfig, AppName), GetConfigDir())
		assert.Equal(t, filepath.Join(xdgConfig, AppName, "config.toml"), GetConfigFile())
		assert.Equal(t, filepath.Join(xdgCache, AppName), GetCacheDir())
	})

	t.Run("EnsureSecureDir sets 0700 permissions", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "nested", "secure_dir")

		err := EnsureSecureDir(target)
		require.NoError(t, err)

		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.True(t, info.IsDir())
		assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
	})
}
