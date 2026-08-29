package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLICommands(t *testing.T) {
	t.Run("version command", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionCmd.SetOut(buf)
		versionCmd.Run(versionCmd, []string{})
		// version outputs via fmt.Printf
	})

	t.Run("config path command", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgFile = filepath.Join(tmpDir, "custom.toml")
		defer func() { cfgFile = "" }()

		configPathCmd.Run(configPathCmd, []string{})
	})

	t.Run("config init command", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgFile = filepath.Join(tmpDir, "init_test.toml")
		defer func() { cfgFile = "" }()

		err := configInitCmd.RunE(configInitCmd, []string{})
		require.NoError(t, err)

		// Second init should fail because file exists
		err = configInitCmd.RunE(configInitCmd, []string{})
		assert.Error(t, err)
	})
}
