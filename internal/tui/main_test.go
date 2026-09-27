package tui_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The model builds an mpv player, which creates a sockets dir under the config
// dir, so the whole package runs against throwaway XDG dirs.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "xdg-test-")
	if err != nil {
		panic(err)
	}
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		if setErr := os.Setenv(v, filepath.Join(dir, v)); setErr != nil {
			panic(setErr)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
