package config

import (
	"os"
	"path/filepath"
)

const (
	AppName        = "torbox-trakt-wrapper"
	DirPermission  = 0700
	FilePermission = 0600
)

func GetConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", AppName)
	}
	return filepath.Join(home, ".config", AppName)
}

func GetConfigFile() string {
	return filepath.Join(GetConfigDir(), "config.toml")
}

func GetCacheDir() string {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".cache", AppName)
	}
	return filepath.Join(home, ".cache", AppName)
}

func GetStateDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".local", "state", AppName)
	}
	return filepath.Join(home, ".local", "state", AppName)
}

func GetLogFile() string {
	return filepath.Join(GetStateDir(), "tt-wrapper.log")
}

func EnsureSecureDir(dir string) error {
	return os.MkdirAll(dir, DirPermission)
}
