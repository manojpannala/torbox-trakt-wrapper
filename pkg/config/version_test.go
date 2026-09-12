package config

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveVersion(t *testing.T) {
	withVersion := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}

	t.Run("go install module@version supplies the version", func(t *testing.T) {
		assert.Equal(t, "v0.2.1", resolveVersion("dev", withVersion("v0.2.1")))
	})

	t.Run("ldflags win when goreleaser stamped one", func(t *testing.T) {
		assert.Equal(t, "v0.2.1", resolveVersion("v0.2.1", withVersion("v9.9.9")))
	})

	t.Run("a plain go build reports devel and stays dev", func(t *testing.T) {
		assert.Equal(t, "dev", resolveVersion("dev", withVersion("(devel)")))
	})

	t.Run("no build info at all", func(t *testing.T) {
		assert.Equal(t, "dev", resolveVersion("dev", nil))
	})
}
